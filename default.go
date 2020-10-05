package doppel

import (
	"html/template"
	"time"

	"github.com/pkg/errors"
)

// defaultCache supports package-level template composition and
// caching.
var defaultCache *Doppel

// Initialize starts the default, package-level cache. Attempting
// to perform operations like Get on the default cache before
// it is initialized will return an error.
//
// The user is responsible for closing the default cache using
// Shutdown or Done when finished.
func Initialize(schematic CacheSchematic, opts ...CacheOption) error {
	if defaultCache != nil { // TODO: Test
		select {
		case <-defaultCache.done:
		default:
			return ErrAlreadyInitialized
		}
	}
	var err error
	defaultCache, err = New(schematic, opts...)
	return err
}

// ErrAlreadyInitialized is returned when the user attempts to
// call Initialize when the default cache is already running.
var ErrAlreadyInitialized = errors.New("the default cache is already running")

// Get returns a copy of the name template if it exists in the cache,
// or an error if it does not.
//
// If Get is called before Initialize, ErrNotInitialized is returned.
func Get(name string) (*template.Template, error) {
	if defaultCache == nil {
		return nil, ErrNotInitialized // TODO: wrap error at boundary
	}

	return defaultCache.Get(name)
}

// ErrNotInitialized is returned when a Get request is made to the
// default cache before Initialize is called.
var ErrNotInitialized = errors.New("Get was called before initializing the default cache")

// Shutdown signals to Get that it should immediately stop accepting
// new requests. It then waits for gracePeriod to elapse before
// closing the request stream. If any requests are still active when
// the request stream is closed, Get will panic.
func Shutdown(gracePeriod time.Duration) {
	defaultCache.Shutdown(gracePeriod)
}

// Close forces the default cache to shut down without accepting
// pending requests. When pending requests are subsequently sent to
// the request stream, Get will panic.
func Close() {
	defaultCache.Close()
}
