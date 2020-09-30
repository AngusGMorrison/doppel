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
	done                  chan struct{}
	requestStream         chan *request
	requestTimeoutSeconds time.Duration // TODO: Implement timeout functional option
	schematic             CacheSchematic
	err                   error
	heartbeat             chan struct{}
	pulseInterval         time.Duration
}

// A CacheSchematic is a collection of named TemplateSchematics,
// allowing TemplateSchematics to reference their base templates by
// name.
//
// TODO: Is this the best representation? Would something more
// explicitly graph-like be an improvement?
type CacheSchematic map[string]*TemplateSchematic

func (cs CacheSchematic) clone() CacheSchematic {
	dest := make(CacheSchematic, len(cs))
	for k, v := range cs {
		dest[k] = v.clone()
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

func (ts *TemplateSchematic) clone() *TemplateSchematic {
	dest := &TemplateSchematic{
		BaseTmplName: ts.BaseTmplName,
		Filepaths:    make([]string, len(ts.Filepaths)),
	}
	copy(dest.Filepaths, ts.Filepaths)
	return dest
}

// New configures a new *Doppel and returns it to the caller.
func New(schematic CacheSchematic, opts ...Option) *Doppel {
	// TODO: Ensure schematic is acylcic.
	d := &Doppel{
		schematic: schematic.clone(), // prevent race conditions as a result of external access
	}
	// TODO: Functional options for pulse rate, timeout...
	for _, opt := range opts {
		d = opt(d)
	}

	done := make(chan struct{})
	if d.done == nil {
		d.done = done
	}

	d.startCache()
	return d
}

type request struct {
	ctx          context.Context
	name         string
	resultStream chan<- *result
}

type result struct {
	tmpl *template.Template
	err  error
}

// cacheTemplates is a concurrent, non-blocking cache or templates and
// sub-templates that runs until cancelled.
//
// If an error is generated when attempting to retrieve a template,
// further requests for that template will return the original error.
//
// Each request to the cache is preemptible via its context.
//
// TODO: Implement interval heartbeat.
// TODO: Implement template expiry.
func (d *Doppel) startCache() {
	// Create heartbeat and request stream synchronously to ensure
	// a caller can never receive nil channels.
	d.heartbeat = make(chan struct{}, 1)
	d.requestStream = make(chan *request)

	go func() {
		defer close(d.heartbeat)
		defer close(d.requestStream)

		templates := make(map[string]*cacheEntry)
		for {
			select {
			case d.heartbeat <- struct{}{}:
				// Signals that cache is ready to receive work.
			default:
			}

			select {
			case <-d.done:
				return
			case req := <-d.requestStream:
				select {
				case <-req.ctx.Done():
					req.resultStream <- &result{err: req.ctx.Err()}
					continue
				default:
				}

				entry := templates[req.name]
				if entry == nil {
					tmplSchematic := d.schematic[req.name]
					if tmplSchematic == nil {
						req.resultStream <- &result{
							// TODO: Improve error wrapping
							err: errors.New(
								fmt.Sprintf("requested schematic %q not found", req.name)),
						}
						continue
					}

					entry = &cacheEntry{ready: make(chan struct{})}
					templates[req.name] = entry
					go entry.parse(req, tmplSchematic)
				} else if entry.err != nil {
					req.resultStream <- &result{err: entry.err}
					continue
				}
				go entry.deliver(req)
			}
		}
	}()
}

// Get returns a named template from the cache. Thread-safe.
func (d *Doppel) Get(name string) (*template.Template, error) {
	select {
	case <-d.done:
		return nil, errors.New("cache has been closed") // TODO: wrap error at package boundary
	default:
	}

	var ctx context.Context
	if d.requestTimeoutSeconds > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), d.requestTimeoutSeconds)
		defer cancel()
	} else {
		ctx = context.Background()
	}

	// Buffer resultStream to ensure timeout-related errors can
	// be sent by the cache even after Get returns.
	resultStream := make(chan *result, 1)
	req := &request{ctx, name, resultStream}

	select {
	case <-ctx.Done():
		return nil, ctx.Err() // TODO: Wrap
	case d.requestStream <- req:
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err() // TODO: Wrap
	case res := <-resultStream:
		if res.err != nil {
			return nil, res.err // TODO: wrap error at package boundary
		}
		return res.tmpl, nil
	}
}

// Heartbeat returns the Doppel's heartbeat channel, which is never
// nil.
func (d *Doppel) Heartbeat() <-chan struct{} {
	return d.heartbeat
}

// Close waits for the current Get request to complete before closing
// the Doppel's done channel. Subsequent requests to the Doppel will
// return ErrDoppelClosed.
//
// TODO: Implement ErrDoppelClosed.
func (d *Doppel) Close() {
	close(d.done)
}
