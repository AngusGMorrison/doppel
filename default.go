package doppel

import (
	"html/template"

	"github.com/pkg/errors"
)

// defaultCache supports package-level template composition and
// caching.
var defaultCache *Doppel

// Initialize starts the default, package-level cache and sets the
// requestStream by which requests are sent to the cache. Attempting
// to perform operations like Get on the default cache before
// it is initialized will return an error.
//
// The user is responsible for closing the default cache via the
// supplied done channel when finished.
func Initialize(done chan struct{}, schematic CacheSchematic, opts ...Option) {
	cancelOpt := func(d *Doppel) *Doppel {
		d.done = done
		return d
	}

	defaultCache = New(schematic, append(opts, cancelOpt)...)
}

// Get returns a copy of the name template if it exists in the cache,
// or an error if it does not.
//
// If Get is called before Initialize, an error is returned.
func Get(name string) (*template.Template, error) {
	if defaultCache == nil {
		return nil, errors.New("Get was called before initializing the default cache") // TODO: wrap error at boundary
	}

	return defaultCache.Get(name)
}
