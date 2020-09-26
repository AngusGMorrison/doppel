// Package templates handles template parsing and access for FB05
// via a concurrent, non-blocking cache.
package templates

import (
	"angusgmorrison/fb05/pkg/env"
	"fmt"
	"html/template"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// InitCache starts the template cache and sets the requestStream by
// which requests are sent to the cache. Must be called before
// attempting to call Get.
func InitCache(done <-chan struct{}, templateDir string) {
	requestStream = cacheTemplates(done, templateDir)
}

type templateSchematic struct {
	baseTemplate string
	files        []string
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

// requestStream is the channel by which the cache receives requests
// for templates.
var requestStream chan<- *request

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
func cacheTemplates(done <-chan struct{}, templateDir string) chan<- *request {
	var (
		sharedDir = filepath.Join(templateDir, "shared")
		publicDir = filepath.Join(templateDir, "public")
	)

	schematics := map[string]*templateSchematic{
		"root": {
			"",
			[]string{filepath.Join(sharedDir, "application.gohtml"),
				filepath.Join(sharedDir, "banner_nav.gohtml")},
		},
		"primary_sidebar_base": {"root", []string{filepath.Join(sharedDir, "sidebar_nav.gohtml")}},
		"homepage":             {"primary_sidebar_base", []string{filepath.Join(publicDir, "homepage.gohtml")}},
		"login":                {"primary_sidebar_base", []string{filepath.Join(publicDir, "login.gohtml")}},
	}

	templates := make(map[string]*cacheEntry)
	requestStream := make(chan *request)
	go func() {
		defer func() {
			close(requestStream)
			requestStream = nil // used by Get to signal an uninitialized cache
		}()

		for {
			select {
			case <-done:
				return
			case req := <-requestStream:
				select {
				case <-req.ctx.Done():
					req.resultStream <- &result{err: req.ctx.Err()}
					continue
				default:
				}

				entry := templates[req.name]
				if entry == nil {
					schematic := schematics[req.name]
					if schematic == nil {
						req.resultStream <- &result{
							err: errors.New(
								fmt.Sprintf("requested schematic %q not found", req.name)),
						}
						continue
					}

					entry = &cacheEntry{ready: make(chan struct{})}
					templates[req.name] = entry
					go entry.parse(req, schematic)
				} else if entry.err != nil {
					req.resultStream <- &result{err: entry.err}
					continue
				}
				go entry.deliver(req)
			}
		}
	}()

	return requestStream
}

type cacheEntry struct {
	ready chan struct{}
	tmpl  *template.Template
	err   error
}

func (ce *cacheEntry) parse(req *request, s *templateSchematic) {
	defer close(ce.ready)

	var err error
	select {
	case <-req.ctx.Done():
		ce.deliverErr(req.ctx.Err(), req)
	default:
	}

	var tmpl *template.Template
	if s.baseTemplate == "" {
		tmpl, err = template.ParseFiles(s.files...)
	} else {
		base, err := Get(s.baseTemplate)
		if err != nil {
			ce.err = err
			return
		}
		tmpl, err = base.ParseFiles(s.files...)
	}
	if err != nil {
		ce.err = err
		return
	}

	ce.tmpl = tmpl
	return
}

func (ce *cacheEntry) deliver(req *request) {
	select {
	case <-req.ctx.Done():
		ce.deliverErr(req.ctx.Err(), req)
	case <-ce.ready:
	}

	if ce.err != nil {
		req.resultStream <- &result{err: ce.err}
		return
	}

	clone, err := ce.tmpl.Clone()
	if err != nil {
		req.resultStream <- &result{err: ce.err}
		return
	}
	req.resultStream <- &result{tmpl: clone}
	return
}

func (ce *cacheEntry) deliverErr(err error, req *request) {
	ce.err = err
	req.resultStream <- &result{err: err}
}

// Get returns a named template from the cache. Thread-safe.
func Get(name string) (*template.Template, error) {
	if requestStream == nil {
		return nil, errors.New("template cache has not been initialized") // TODO: wrap error at package boundary
	}

	timeout := env.GetDuration("TEMPLATE_TIMEOUT") * time.Second // TODO: check presence of env var before using
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resultStream := make(chan *result)
	req := &request{ctx, name, resultStream}
	requestStream <- req
	res := <-resultStream
	if res.err != nil {
		return nil, res.err // TODO: wrap error at package boundary
	}
	return res.tmpl, nil
}
