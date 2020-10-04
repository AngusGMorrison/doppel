package doppel

import (
	"html/template"

	"github.com/pkg/errors"
)

type cacheEntry struct {
	ready chan struct{}
	tmpl  *template.Template
	err   error
}

func (ce *cacheEntry) parse(req *request, s *TemplateSchematic, d *Doppel) {
	defer close(ce.ready)

	var err error
	select {
	case <-req.ctx.Done():
		ce.err = errParseTimeout
		req.resultStream <- &result{err: req.ctx.Err()}
		return
	default:
	}

	var tmpl *template.Template
	if s.BaseTmplName == "" {
		tmpl, err = template.ParseFiles(s.Filepaths...)
	} else {
		base, err := d.Get(s.BaseTmplName)
		if err != nil {
			ce.err = err
			return
		}
		tmpl, err = base.ParseFiles(s.Filepaths...)
	}
	if err != nil {
		ce.err = err
		return
	}

	ce.tmpl = tmpl
}

func (ce *cacheEntry) deliver(req *request) {
	select {
	case <-req.ctx.Done():
		req.resultStream <- &result{err: req.ctx.Err()}
		return
	case <-ce.ready:
	}

	if ce.err != nil {
		req.resultStream <- &result{err: ce.err}
		return
	}

	// Return a copy of the template that can be safely executed
	// without affecting cached templates.
	clone, err := ce.tmpl.Clone()
	if err != nil {
		req.resultStream <- &result{err: ce.err}
		return
	}
	req.resultStream <- &result{tmpl: clone}
}

// errParseTimeout signals to the cache that parsing may be
// successfully retried.
var errParseTimeout = errors.New("timed out while parsing template")
