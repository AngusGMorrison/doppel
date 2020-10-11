package doppel

import (
	"context"
	"fmt"
	"html/template"

	"github.com/pkg/errors"
)

type cacheEntry struct {
	ready     chan struct{}
	retry     chan struct{}
	schematic *TemplateSchematic
	tmpl      *template.Template
	err       error
}

func (ce *cacheEntry) setErr(err error, retryTimeouts bool) {
	if err == context.Canceled || retryTimeouts && err == context.DeadlineExceeded {
		select {
		case ce.retry <- struct{}{}:
		default:
		}
		return
	}
	ce.err = err
	close(ce.ready)
}

func (d *Doppel) parse(ce *cacheEntry, req *request) {
	select {
	case <-req.ctx.Done():
		ce.setErr(req.ctx.Err(), d.retryTimeouts)
		return
	default:
	}

	if ce.schematic == nil {
		msg := fmt.Sprintf(logMissingSchematic, req.name)
		d.log.Printf(msg)
		ce.setErr(errors.New(msg), d.retryTimeouts) // TODO: Improve error
		return
	}

	var tmpl *template.Template
	var err error
	if ce.schematic.BaseTmplName == "" {
		tmpl, err = template.ParseFiles(ce.schematic.Filepaths...)
	} else {
		// Synchronize recursive requests with the original Get's timeout or
		// cancellation. req's context can't simply be wrapped by the new one
		// because it is a struct field that hasn't flowed down the call stack
		// in the usual fashion.
		d.log.Printf(logGettingBaseTemplate, ce.schematic.BaseTmplName, req.name)
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			<-req.ctx.Done() // guaranteed to be closed when the parent Get returns
			// TODO: Test this guarantee.
			cancel()
		}()

		base, err := d.Get(ctx, ce.schematic.BaseTmplName)
		if err != nil {
			ce.setErr(err, d.retryTimeouts)
			return
		}
		tmpl, err = base.ParseFiles(ce.schematic.Filepaths...)
	}

	if err != nil {
		d.log.Printf(logParsingError, req.name)
		ce.setErr(err, d.retryTimeouts)
		return
	}
	d.log.Printf(logParsingSuccess, req.name)
	ce.tmpl = tmpl
	// TODO: How to ensure close is never called
	close(ce.ready)
}

func (d *Doppel) deliver(ce *cacheEntry, req *request) {
loop:
	for {
		select {
		case <-req.ctx.Done():
			d.log.Printf(logRequestInterrupted, req.name)
			return
		case <-ce.retry:
			go d.parse(ce, req)
		case <-ce.ready:
			break loop
		}
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
