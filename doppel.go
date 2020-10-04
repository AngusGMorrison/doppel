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

// New configures a new *Doppel and returns it to the caller. It
// should not be used concurrently with operations on the provided
// schematic.
func New(schematic CacheSchematic, opts ...CacheOption) (*Doppel, error) {
	if cyclic, err := IsCyclic(schematic); cyclic {
		return nil, err // TODO: Wrap
	}

	d := &Doppel{
		schematic: schematic.Clone(), // prevent race conditions as a result of external access
	}

	// TODO: Functional options for timeout...
	for _, opt := range opts {
		opt(d)
	}

	if d.done == nil {
		d.done = make(chan struct{})
	}

	d.startCache()
	return d, nil
}

type request struct {
	ctx          context.Context
	name         string
	resultStream chan<- *result
	noCache      bool // TODO: test
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
		defer close(d.requestStream)

		templates := make(map[string]*cacheEntry)

		for {
			select {
			case d.heartbeat <- struct{}{}:
				// Signals that cache is at the top of its work loop.
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
				if entry == nil || entry.err == errParseTimeout || req.noCache {
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
					go entry.parse(req, tmplSchematic, d)
				} else if entry.err != nil {
					req.resultStream <- &result{err: entry.err}
					continue
				}
				go entry.deliver(req)
			}
		}
	}()
}

// Get returns a named template from the cache. Get is thread-safe.
func (d *Doppel) Get(name string) (*template.Template, error) {
	select {
	case <-d.done:
		return nil, ErrDoppelClosed // TODO: wrap error at package boundary
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
	req := &request{ctx, name, resultStream, false}

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

// Heartbeat returns the Doppel's heartbeat channel, which is
// guaranteed to be non-nil.
func (d *Doppel) Heartbeat() <-chan struct{} {
	return d.heartbeat
}

// Close immediately closes the Doppel's done channel. The Doppel's
// cache will complete the request currently in progress (if any)
// before returning. Subsequent requests to the Doppel will return
// ErrDoppelClosed.
func (d *Doppel) Close() {
	close(d.done)
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
			if ts := cs[name]; ts != nil {
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
