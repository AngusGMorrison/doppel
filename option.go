package doppel

import "time"

// CacheOption are used to decorate new Doppels, e.g. adding template
// expiry or memory limits.
type CacheOption func(*Doppel)

// WithGlobalTimeout returns a CacheOption that sets a maximum
// runtime for all requests made to the Doppel.
func WithGlobalTimeout(timeout time.Duration) CacheOption {
	return func(d *Doppel) {
		d.globalTimeout = timeout
	}
}

// WithLogger allows the user to specify a logger to be embedded in the Doppel.
func WithLogger(log logger) CacheOption {
	return func(d *Doppel) {
		d.log = log
	}
}

// WithTimeoutRetry causes cache entries that have entered an error state as
// a result of request timeout to be retried.
func WithTimeoutRetry() CacheOption {
	return func(d *Doppel) {
		d.timeoutRetry = true // TODO: implement
	}
}

// RequestOption allows configuration of individual Get requests.
type RequestOption func(*request)

// WithCacheRefresh forces the cached result to be reparsed.
func WithCacheRefresh() RequestOption {
	return func(r *request) {
		r.refreshCache = true // TODO: implement
	}
}

// TODO: Implement stale template expiry.
// func WithExpiry(expireAfter time.Duration) Option {

// }

// TODO - stretch: Implement memory limit.
// func WithMemoryLimit(limitInMB uint64) Option {

// }
