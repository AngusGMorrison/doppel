package doppel

import (
	"context"
	"html/template"
)

type cacheEntry struct {
	ready chan struct{}
	tmpl  *template.Template
	err   error
}

func (ce *cacheEntry) shouldRetry(req *request) bool {
	return ce.err == context.DeadlineExceeded ||
		ce.err == context.Canceled ||
		req.noCache
}

func (ce *cacheEntry) parse(req *request, s *TemplateSchematic, d *Doppel) {
	defer close(ce.ready)

	select {
	case <-req.done:
		ce.err = ErrRequestTimeout
		return
	default:
	}

	var tmpl *template.Template
	var err error
	if s.BaseTmplName == "" {
		tmpl, err = template.ParseFiles(s.Filepaths...)
	} else {
		base, err := d.Get(s.BaseTmplName) // TODO: Secondary request is not beholden to the timeout of the first.
		if err != nil {
			ce.err = err
			return
		}

		select {
		case <-req.done:
			ce.err = ErrRequestTimeout
			return
		default:
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
	case <-req.done:
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
