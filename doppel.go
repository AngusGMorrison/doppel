// Package doppel provides configurable, concurrent, non-blocking
// caching of composable templates.
package doppel

import (
	"fmt"
	"html/template"
	"time"

	"github.com/pkg/errors"
)

// A Doppel provides a mechanism to configure, send requests to and
// close a concurrent, non-blocking template cache.
//
// A template is parsed when it is first requested, recursively
// parsing any templates on which it depends if they are not present
// in the cache. Parsed templates are stored in memory until the
// program ends, a timeout expires, or a memory threshold has been
// reached, per user configuration via functional options.
type Doppel struct {
	globalTimeout time.Duration
	schematic     CacheSchematic
	heartbeat     chan struct{} // signals the start of each work loop
	requestStream chan *request // sends requests to the work loop
	inShutdown    chan struct{} // signals that graceful shutdown has been triggered
	done          chan struct{} // signals that the work loop has returned
	log           logger
}

// A CacheSchematic is an acyclic graph of named TemplateSchematics
// which reference their base templates by name.
type CacheSchematic map[string]*TemplateSchematic

// Clone returns a deep copy of the CacheSchematic.
func (cs CacheSchematic) Clone() CacheSchematic {
	dest := make(CacheSchematic, len(cs))
	for k, v := range cs {
		dest[k] = v.Clone()
	}
	return dest
}

// TemplateSchematic describes how to parse a template from a named
// base template in the cache and zero or more template files.
//
// BaseTmplName may be an empty string, indicating a template without
// a base.
type TemplateSchematic struct {
	BaseTmplName string
	Filepaths    []string
}

// Clone returns a pointer to deep copy of the underlying
// TemplateSchematic.
func (ts *TemplateSchematic) Clone() *TemplateSchematic {
	dest := &TemplateSchematic{
		BaseTmplName: ts.BaseTmplName,
		Filepaths:    make([]string, len(ts.Filepaths)),
	}
	copy(dest.Filepaths, ts.Filepaths)
	return dest
}

type logger interface {
	Printf(fmt string, args ...interface{})
}

// defaultLog provides a no-op logger to avoid a series of nil checks throughout
// the cache's work loop.
type defaultLog struct{}

func (d *defaultLog) Printf(fmt string, args ...interface{}) {
	// No-op.
}

// New configures a new *Doppel and returns it to the caller. It
// should not be used concurrently with operations on the provided
// schematic.
func New(schematic CacheSchematic, opts ...CacheOption) (*Doppel, error) {
	if cyclic, err := IsCyclic(schematic); cyclic {
		return nil, err // TODO: Wrap
	}

	d := &Doppel{
		schematic:  schematic.Clone(), // prevent race conditions as a result of external access
		inShutdown: make(chan struct{}),
		done:       make(chan struct{}),
	}

	for _, opt := range opts {
		opt(d)
	}

	if d.log == nil {
		d.log = &defaultLog{}
	}

	d.startCache()
	return d, nil
}

type request struct {
	name         string          // the name of the template to fetch
	cancel       <-chan struct{} // provided by the user via the WithCancel RequestOption
	timeout      time.Duration   // provided by the user via WithTimeout or WithGlobalTimeout
	done         <-chan struct{} // closed by Get when the request should be canceled or times out
	resultStream chan<- *result  // used by Get to receive results from the cache
	noCache      bool            // disable caching for the request // TODO: test
}

type result struct {
	tmpl *template.Template
	err  error
}

// startCache launches a concurrent, non-blocking cache of templates
// and sub-templates that runs until cancelled.
//
// If an error is generated when attempting to retrieve a template,
// further requests for that template will return the original error.
//
// Each request to the cache is preemptible via its context.
// TODO: Implement template expiry.
func (d *Doppel) startCache() {
	// Create heartbeat and request stream synchronously to ensure
	// a caller can never receive nil channels.
	d.heartbeat = make(chan struct{}, 1)
	d.requestStream = make(chan *request)

	go func() {
		defer close(d.heartbeat)
		defer close(d.done)

		templates := make(map[string]*cacheEntry)
		for req := range d.requestStream {
			d.log.Printf("received request for template %q", req.name)
			select {
			case d.heartbeat <- struct{}{}:
				// Signals that cache is at the top of its work loop.
			default:
			}

			select {
			case <-req.done:
				d.log.Printf("request for template %q cancelled", req.name)
				continue
			default:
			}

			entry := templates[req.name]
			if entry == nil || entry.shouldRetry(req) {
				d.log.Printf("parsing template %q", req.name)
				tmplSchematic := d.schematic[req.name]
				if tmplSchematic == nil {
					msg := fmt.Sprintf("missing schematic for template %q", req.name)
					d.log.Printf(msg)
					req.resultStream <- &result{err: errors.New(msg)}
					continue
				}

				entry = &cacheEntry{ready: make(chan struct{})}
				templates[req.name] = entry
				go d.parse(entry, req, tmplSchematic)
			}
			go d.deliver(entry, req)
		}
	}()
}

// Get returns a named template from the cache. Get is thread-safe.
// TODO: Make Get take a context for timeout and cancellation.
func (d *Doppel) Get(name string, opts ...RequestOption) (*template.Template, error) {
	select {
	case <-d.inShutdown:
		return nil, ErrDoppelClosed
	default:
	}

	done := make(chan struct{})
	// Buffer resultStream for cases where timeout expires concurrently with results being sent
	resultStream := make(chan *result, 1)
	req := &request{
		done:         done,
		name:         name,
		resultStream: resultStream,
		noCache:      false,
	}

	for _, opt := range opts {
		opt(req)
	}

	// TODO: Consider handling this with a context that is passed in, not
	// part of the request object.x
	if d.globalTimeout > 0 && d.globalTimeout < req.timeout {
		req.timeout = d.globalTimeout
	}

	var timeout <-chan time.Time
	if req.timeout > 0 {
		timeout = time.After(req.timeout)
	}

	select {
	case <-timeout:
		close(done)
		return nil, ErrRequestTimeout
	case <-req.cancel:
		close(done)
		return nil, ErrRequestCanceled
	case d.requestStream <- req:
	}

	select {
	case <-timeout:
		close(done)
		return nil, ErrRequestTimeout
	case <-req.cancel:
		close(done)
		return nil, ErrRequestCanceled
	case res := <-resultStream:
		if res.err != nil {
			return nil, res.err
		}
		return res.tmpl, nil
	}
}

var ErrRequestTimeout = errors.New("request timed out")                // TODO: Improve
var ErrRequestCanceled = errors.New("request cancelled by the caller") // TODO: Improve

// Heartbeat returns the Doppel's heartbeat channel, which is
// guaranteed to be non-nil.
func (d *Doppel) Heartbeat() <-chan struct{} {
	return d.heartbeat
}

// Shutdown signals to Get that it should immediately stop accepting
// new requests. It then waits for gracePeriod to elapse before
// closing the request stream. If any requests are still active when
// the request stream is closed, Get will panic.
func (d *Doppel) Shutdown(gracePeriod time.Duration) {
	close(d.inShutdown) // signals that Get should no longer accept new requests
	d.log.Printf("shutting down gracefully...")
	<-time.After(gracePeriod) // TODO: Create a way of waiting until the request stream is drained.
	close(d.requestStream)
	d.log.Printf("shutdown complete")
}

// Close forces the Doppel to shut down without accepting pending
// requests. When pending requests are subsequently sent to the
// request stream, Get will panic.
func (d *Doppel) Close() {
	close(d.inShutdown)
	close(d.requestStream)
}

// ErrDoppelClosed is returned in response to requests to a Doppel
// with an closed cache.
var ErrDoppelClosed = errors.New("the Doppel's cache has already been closed")

// IsCyclic reports whether a CacheSchematic contains a cycle. If
// true, the accompanying error describes which TemplateSchematics
// form part of the cycle.
func IsCyclic(cs CacheSchematic) (bool, error) {
	seen := make(map[string]bool)
	var recStack []string // track TemplateSchematics already seen in the current traversal

	var visit func(name string) error
	visit = func(name string) error {
		for _, seenName := range recStack {
			if seenName == name {
				msg := fmt.Sprintf("cycle through %s: %v", name, append(recStack, name))
				return errors.New(msg)
			}
		}
		recStack = append(recStack, name)

		var err error
		if !seen[name] {
			seen[name] = true
			if tmplSchematic := cs[name]; tmplSchematic != nil {
				err = visit(cs[name].BaseTmplName)
			}
		}
		recStack = recStack[:len(recStack)-1]
		return err
	}

	for k := range cs {
		if err := visit(k); err != nil {
			return true, err
		}
	}
	return false, nil
}
