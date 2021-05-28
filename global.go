package doppel

import (
	"context"
	"html/template"

	"github.com/pkg/errors"
)

// globalCache supports package-level template composition and
// caching.
var globalCache *Doppel

// Initialize starts the default, global cache. Attempting to perform operations
// like Get on the global cache before it is initialized will return an error.
//
// The user is responsible for closing the global cache via the context param.
func Initialize(ctx context.Context, schematic CacheSchematic, opts ...CacheOption) error {
	if globalCache != nil {
		select {
		case <-globalCache.done:
		default:
			return errors.WithStack(ErrAlreadyInitialized)
		}
	}

	var err error
	globalCache, err = New(ctx, schematic, opts...)
	return err
}

// Get returns a copy of the name template if it exists in the cache,
// or an error if it does not.
//
// If Get is called before Initialize, ErrNotInitialized is returned.
func Get(ctx context.Context, name string) (*template.Template, error) {
	if globalCache == nil {
		return nil, errors.WithStack(ErrNotInitialized)
	}

	return globalCache.Get(ctx, name)
}
