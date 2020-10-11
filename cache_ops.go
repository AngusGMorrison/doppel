package doppel

import (
	"context"
	"fmt"
	"html/template"
	"time"

	"github.com/pkg/errors"
)

type cacheEntry struct {
	ready     chan struct{}      // signals ready to return results
	retry     chan struct{}      // signals to retry parsing in subsequent requests (e.g. after cancelletion)
	schematic *TemplateSchematic // embedded schemaitc enables reparsing if a retry is required
	tmpl      *template.Template // the parsed template
	err       error              // any error encountered while parsing
}

func (ce *cacheEntry) signalStatus(retryTimeouts bool) {
	if errors.Is(ce.err, context.Canceled) || retryTimeouts && errors.Is(ce.err, context.DeadlineExceeded) {
		select {
		case ce.retry <- struct{}{}:
		default:
		}
		return
	}

	close(ce.ready)
}

func (d *Doppel) parse(ce *cacheEntry, req *request) {
	defer ce.signalStatus(d.retryTimeouts)

	select {
	case <-req.ctx.Done():
		ce.err = errors.WithStack(req.ctx.Err())
		return
	default:
	}

	ce.err = nil // reset error in the event of a retry

	if ce.schematic == nil {
		msg := fmt.Sprintf(logMissingSchematic, req.name)
		d.log.Printf(msg)
		ce.err = RequestError{
			errors.WithStack(ErrSchematicNotFound),
			req.name,
			time.Since(req.start),
		}
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
			cancel()
		}()
		base, err := d.Get(ctx, ce.schematic.BaseTmplName)
		if err != nil {
			ce.err = err
			return
		}

		tmpl, err = base.ParseFiles(ce.schematic.Filepaths...)
	}

	if err != nil {
		d.log.Printf(logParsingError, req.name)
		ce.err = RequestError{err, req.name, time.Since(req.start)}
		return
	}
	d.log.Printf(logParsingSuccess, req.name)
	ce.tmpl = tmpl
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
	d.log.Printf(logDeliveringTemplate, req.name)
	clone, _ := ce.tmpl.Clone()
	req.resultStream <- &result{tmpl: clone}
}
