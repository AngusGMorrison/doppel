package doppel

import (
	"context"
	"fmt"
	"html/template"

	"github.com/pkg/errors"
)

type cacheEntry struct {
	ready chan struct{}
	tmpl  *template.Template
	err   error
}

func (d *Doppel) shouldRetry(ce *cacheEntry, req *request) bool {
	return req.refreshCache || d.retryInterrupted && isInterruptErr(ce.err)
}

func isInterruptErr(err error) bool {
	return err == context.DeadlineExceeded ||
		err == context.Canceled ||
		err == errRequestInterrupted
}

func (d *Doppel) parse(ce *cacheEntry, req *request, s *TemplateSchematic) {
	defer close(ce.ready)

	select {
	case <-req.ctx.Done():
		d.log.Printf(logRequestCanceled, req.name) // TODO: Make logs suitable to ctx err
		ce.err = req.ctx.Err()
		return
	default:
	}

	if s == nil {
		msg := fmt.Sprintf(logMissingSchematic, req.name)
		d.log.Printf(msg)
		ce.err = errors.New(msg) // TODO: Improve error
		return
	}

	var tmpl *template.Template
	var err error
	if s.BaseTmplName == "" {
		tmpl, err = template.ParseFiles(s.Filepaths...)
	} else {
		// Synchronize recursive requests with the original Get's timeout or
		// cancellation. req's context can't simply be wrapped by the new one
		// because it is a struct field that hasn't flowed down the call stack
		// in the usual fashion.
		d.log.Printf(logGettingBaseTemplate, s.BaseTmplName, req.name)
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			<-req.ctx.Done() // guaranteed to be closed when the parent Get returns
			// TODO: Test this guarantee.
			cancel()
		}()

		base, err := d.Get(ctx, s.BaseTmplName)
		if err != nil {
			ce.err = err
			return
		}
		tmpl, err = base.ParseFiles(s.Filepaths...)
	}

	if err != nil {
		d.log.Printf(logParsingError, req.name)
		ce.err = err
		return
	}
	d.log.Printf(logParsingSuccess, req.name)
	ce.tmpl = tmpl
}

func (d *Doppel) deliver(ce *cacheEntry, req *request) {
	select {
	case <-req.ctx.Done():
		d.log.Printf(logRequestCanceled, req.name)
		return
	case <-ce.ready:
	}

	if ce.err != nil {
		d.log.Printf(logDeliveringCachedError, req.name)
		req.resultStream <- &result{err: ce.err}
		return
	}

	// Return a copy of the template that can be safely executed
	// without affecting cached templates.
	clone, err := ce.tmpl.Clone()
	if err != nil {
		d.log.Printf(logCloningError, req.name, err)
		req.resultStream <- &result{err: ce.err}
		return
	}
	d.log.Printf(logDeliveringTemplate, req.name)
	req.resultStream <- &result{tmpl: clone}
}
