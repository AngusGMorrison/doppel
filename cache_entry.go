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

func (d *Doppel) parse(ce *cacheEntry, req *request, s *TemplateSchematic) {
	defer close(ce.ready)

	select {
	case <-req.done:
		d.log.Printf("request for template %q cancelled", req.name)
		ce.err = ErrRequestTimeout
		return
	default:
	}

	var tmpl *template.Template
	var err error
	if s.BaseTmplName == "" {
		tmpl, err = template.ParseFiles(s.Filepaths...)
	} else {
		d.log.Printf("fetching base template %q for %q", s.BaseTmplName, req.name)
		base, err := d.Get(s.BaseTmplName) // TODO: Secondary request is not beholden to the timeout of the first.
		if err != nil {
			ce.err = err
			return
		}

		select {
		case <-req.done:
			d.log.Printf("request for template %q cancelled", req.name)
			ce.err = ErrRequestTimeout
			return
		default:
		}

		tmpl, err = base.ParseFiles(s.Filepaths...)
	}
	if err != nil {
		d.log.Printf("parsing error for template %q", req.name)
		ce.err = err
		return
	}

	d.log.Printf("template %q parsed successfully", req.name)
	ce.tmpl = tmpl
}

func (d *Doppel) deliver(ce *cacheEntry, req *request) {
	select {
	case <-req.done:
		d.log.Printf("request for template %q cancelled", req.name)
		return
	case <-ce.ready:
	}

	if ce.err != nil {
		d.log.Printf("found cached error for template %q", req.name)
		req.resultStream <- &result{err: ce.err}
		return
	}

	// Return a copy of the template that can be safely executed
	// without affecting cached templates.
	clone, err := ce.tmpl.Clone()
	if err != nil {
		d.log.Printf("error cloning template %q: %v", req.name, err)
		req.resultStream <- &result{err: ce.err}
		return
	}
	d.log.Printf("delivering template %q", req.name)
	req.resultStream <- &result{tmpl: clone}
}
