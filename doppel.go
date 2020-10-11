// Package doppel provides configurable, concurrent, non-blocking
// caching of composable templates.
package doppel

import (
	"context"
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
	retryTimeouts bool // flags whether to retry parsing templates that have previously timed out
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

func (d *defaultLog) Printf(format string, args ...interface{}) {
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
	name         string         // the name of the template to fetch
	resultStream chan<- *result // used by Get to receive results from the cache

	// While generally inadvisable to store contexts in structs, ctx functions
	// solely as a messenger, informing downstream Get requests when the
	// original request has timed out or been canceled.
	ctx context.Context
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

		cache := make(map[string]*cacheEntry)
		for req := range d.requestStream {
			d.log.Printf(logRequestReceived, req.name)
			select {
			case d.heartbeat <- struct{}{}:
				// Signals that cache is at the top of its work loop.
			default:
			}

			select {
			case <-req.ctx.Done():
				d.log.Printf(logRequestInterrupted, req.name)
				continue
			default:
			}

			entry := cache[req.name]
			if entry == nil {
				d.log.Printf(logParsingTemplate, req.name)
				tmplSchematic := d.schematic[req.name]
				if tmplSchematic != nil {
					tmplSchematic = tmplSchematic.Clone()
				}

				entry = &cacheEntry{
					ready:     make(chan struct{}),
					retry:     make(chan struct{}, 1),
					schematic: tmplSchematic,
				}
				cache[req.name] = entry
				go d.parse(entry, req)
			}
			go d.deliver(entry, req)
		}
	}()
}

// Get returns a named template from the cache. Get is thread-safe and
// can be preempted via the supplied context.Context.
func (d *Doppel) Get(ctx context.Context, name string) (*template.Template, error) {
	select {
	case <-d.inShutdown:
		return nil, ErrDoppelClosed
	default:
	}

	// Buffer resultStream for cases where timeout expires concurrently with results being sent.
	resultStream := make(chan *result, 1)
	req := &request{
		// done:         done,
		name:         name,
		resultStream: resultStream,
	}

	if d.globalTimeout > 0 {
		var cancel context.CancelFunc
		// WithTimeout retains the the parent context's timeout if
		// d.globalTimeout occurs later.
		ctx, cancel = context.WithTimeout(ctx, d.globalTimeout)
		defer cancel()
	}

	// Wrap ctx to enforce cancellation of recursive Get requests if the
	// original request returns early (e.g. due to timeout).
	ctx, cancel := context.WithCancel(ctx)
	req.ctx = ctx
	defer cancel()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case d.requestStream <- req:
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-resultStream:
		if res.err != nil {
			return nil, res.err
		}
		return res.tmpl, nil
	}
}

// Heartbeat returns the Doppel's heartbeat channel, which is guaranteed to be
// non-nil.
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
	go func() {
		<-time.After(gracePeriod) // TODO: Create a way of waiting until the request stream is drained.
		close(d.requestStream)
		d.log.Printf("shutdown complete")
	}()
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
