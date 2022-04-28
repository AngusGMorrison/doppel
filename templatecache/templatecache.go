// Package templatecache provides a concurrent, non-blocking cache of composable
// templates.
package templatecache

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"text/template"
)

var (
	ErrParsingFailed = errors.New("failed to parse template")
	ErrCacheClosed   = errors.New("cache was closed")
)

// request signals a template request to the cache.
type request struct {
	templateName Name
	resultStream chan<- result
	// ctx allows for request preemption (e.g., cancellation of the template
	// request if the HTTP request that triggers it is cancelled).
	ctx context.Context
}

// result is the cache's response to a request. It may contain either a template
// or an error.
type result struct {
	template *template.Template
	err      error
}

// Cache represents a threadsafe, non-blocking cache of lazily compiled
// templates.
type Cache struct {
	schema CacheSchema
	// The channel on which requests for templates are received.
	requestStream chan request
	done          <-chan struct{}

	// once is used to ensure that requestStream is drained and closed only once
	// on cache shutdown to avoid panics from closing a closed channel.
	once sync.Once
}

// New returns a running cache to the caller.
func New(ctx context.Context, schema CacheSchema) *Cache {
	requestStream := make(chan request)

	cache := &Cache{
		// Clone the schema to prevent concurrent access to the original schema.
		schema:        schema.Clone(),
		requestStream: requestStream,
		done:          ctx.Done(),
	}

	cache.start()

	return cache
}

type cacheEntry struct {
	// Ready signals to waiting goroutines that the template has been parsed.
	ready chan struct{}

	// Retry signals that parsing should be retried, e.g. if a previous request
	// for the template was canceled. Retry should be buffered so sending
	// doesn't block template-parsing goroutines from exiting.
	retry chan struct{}

	schema   TemplateSchema
	template *template.Template
	err      error
}

func newCacheEntry(schema TemplateSchema) *cacheEntry {
	return &cacheEntry{
		ready:  make(chan struct{}),
		retry:  make(chan struct{}, 1),
		schema: schema,
	}
}

func (c *Cache) start() {
	go func() {
		// Internally, the cache is represented as a map which is isolated
		// within a goroutine, ensuring serial access.
		cache := make(map[Name]*cacheEntry)

		select {
		case <-c.done:
			// TODO: Drain request channel
			return
		default:
		}

		// Listen on requestStream and handle requests for templates.
		for req := range c.requestStream {
			select {
			case <-c.done:
				// Cache has been canceled; shutdown.
				req.resultStream <- result{err: ErrCacheClosed}

				return
			case <-req.ctx.Done():
				// Request has been canceled; handle the next request.
				req.resultStream <- result{err: req.ctx.Err()}

				continue
			default:
			}

			entry := cache[req.templateName]
			if entry == nil {
				templateSchema := c.schema.graph[req.templateName]
				entry = newCacheEntry(templateSchema)
				cache[req.templateName] = entry
				go c.parseTemplate(entry, req)
			}

			// go c.deliver(entry, req)

		}
	}()
}

func (c *Cache) parseTemplate(entry *cacheEntry, req request) {
	select {
	case <-c.done:
		// Cache closure is a non-retryable error.
		entry.err = ErrCacheClosed
		close(entry.ready)

		return
	case <-req.ctx.Done():
		// Request cancellation or expiry is a retryable error.
		entry.err = fmt.Errorf("request was canceled: %w", req.ctx.Err())
		entry.retry <- struct{}{}

		return
	default:
	}

	var (
		tmpl *template.Template
		err  error
	)

	if entry.schema.Parent == "" {
		tmpl, err = template.ParseFiles(entry.schema.ComponentPaths...)
	} else {
		tmpl, err = c.getParentTemplate(entry, req)
	}

	if err != nil {
		// Parsing errors are non-retryable.
		entry.err = fmt.Errorf("%w: %v", ErrParsingFailed, err)
		close(entry.ready)

		return
	}

	entry.template = tmpl
	close(entry.ready)
}

func (c *Cache) getParentTemplate(entry *cacheEntry, req request) (*template.Template, error) {
	parentTmpl, err := c.Get(req.ctx, entry.schema.Parent)
	if err != nil {
		entry.err = err

		if retryable(err) {
			entry.retry <- struct{}{}
		}
	}

	return parentTmpl.ParseFiles(entry.schema.ComponentPaths...)
}

func retryable(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// Get returns the compiled template for the given name. Get returns an error if
// the cache is closed, the provided context expires or the template can't be
// parsed.
func (c *Cache) Get(ctx context.Context, name Name) (*template.Template, error) {
	// Check for cache closure and request expiry before making a new request.
	select {
	case <-c.done:
		return nil, ErrCacheClosed
	case <-ctx.Done():
		return nil, fmt.Errorf("request was canceled: %w", ctx.Err())
	default:
	}

	resultStream := make(chan result)
	req := request{
		templateName: name,
		resultStream: resultStream,
		ctx:          ctx,
	}

	// Send the request only if the cache and context are live. If the request
	// send happens simultaneously with one or more of the other events and the
	// request send case is selected, request preemption will still occur within
	// the cache loop.
	select {
	case <-c.done:
		return nil, ErrCacheClosed
	case <-ctx.Done():
		return nil, fmt.Errorf("request was canceled: %w", ctx.Err())
	case c.requestStream <- req:
	}

	// Await the result. Request premption is handled within the cache loop.
	result := <-resultStream

	return result.template, result.err
}
