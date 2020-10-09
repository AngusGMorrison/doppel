package doppel

import (
	"context"
	"html/template"
	"time"

	"github.com/pkg/errors"
)

// globalCache supports package-level template composition and
// caching.
var globalCache *Doppel

// Initialize starts the default, global cache. Attempting
// to perform operations like Get on the global cache before
// it is initialized will return an error.
//
// The user is responsible for closing the global cache using
// Shutdown or Done when finished.
func Initialize(schematic CacheSchematic, opts ...CacheOption) error {
	if globalCache != nil {
		select {
		case <-globalCache.done:
		default:
			return ErrAlreadyInitialized
		}
	}
	var err error
	globalCache, err = New(schematic, opts...)
	return err
}

// ErrAlreadyInitialized is returned when the user attempts to
// call Initialize when the global cache is already running.
var ErrAlreadyInitialized = errors.New("the global cache is already running")

// Get returns a copy of the name template if it exists in the cache,
// or an error if it does not.
//
// If Get is called before Initialize, ErrNotInitialized is returned.
func Get(ctx context.Context, name string) (*template.Template, error) {
	if globalCache == nil {
		return nil, ErrNotInitialized // TODO: wrap error at boundary
	}

	return globalCache.Get(ctx, name)
}

// ErrNotInitialized is returned when a Get request is made to the
// global cache before Initialize is called.
var ErrNotInitialized = errors.New("Get was called before initializing the global cache")

// Shutdown signals to Get that it should immediately stop accepting
// new requests. It then waits for gracePeriod to elapse before
// closing the request stream. If any requests are still active when
// the request stream is closed, Get will panic.
func Shutdown(gracePeriod time.Duration) {
	globalCache.Shutdown(gracePeriod)
}

// Close forces the global cache to shut down without accepting
// pending requests. When pending requests are subsequently sent to
// the request stream, Get will panic.
func Close() {
	globalCache.Close()
}
