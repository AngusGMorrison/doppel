package doppel

import "html/template"

type cacheEntry struct {
	ready chan struct{}
	tmpl  *template.Template
	err   error
}

func (ce *cacheEntry) parse(req *request, s *TemplateSchematic) {
	defer close(ce.ready)

	var err error
	select {
	case <-req.ctx.Done():
		ce.deliverErr(req.ctx.Err(), req)
	default:
	}

	var tmpl *template.Template
	if s.BaseTmplName == "" {
		tmpl, err = template.ParseFiles(s.Filepaths...)
	} else {
		base, err := Get(s.BaseTmplName)
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
