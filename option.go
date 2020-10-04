package doppel

import "time"

// CacheOption are used to decorate new Doppels, e.g. adding template
// expiry or memory limits.
type CacheOption func(*Doppel) // TODO: Ensure custom Options can't affect unexported fields.

// WithGlobalTimeout returns a CacheOption that sets a maximum
// runtime for all requests made to the Doppel.
func WithGlobalTimeout(timeout time.Duration) CacheOption {
	return func(d *Doppel) {
		d.globalTimeout = timeout
	}
}

// TODO: Implement stale template expiry.
// func WithExpiry(expireAfter time.Duration) Option {

// }

// TODO - stretch: Implement memory limit.
// func WithMemoryLimit(limitInMB uint64) Option {

// }
