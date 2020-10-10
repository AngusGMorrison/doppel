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

const (
	logRequestReceived       = "received request for template %q"
	logRequestCanceled       = "request for template %q canceled"
	logParsingTemplate       = "parsing template %q"
	logMissingSchematic      = "missing schematic for template %q"
	logGettingBaseTemplate   = "getting base template %q for %q"
	logParsingError          = "parsing error for template %q"
	logParsingSuccess        = "template %q parsed successfully"
	logDeliveringCachedError = "delivering cached error for template %q"
	logCloningError          = "error cloning template %q: %v"
	logDeliveringTemplate    = "delivering template %q"
)

// WithRetryInterrupted causes cache entries in an error state as a result of
// timeout or cancellation to be retried.
func WithRetryInterrupted() CacheOption {
	return func(d *Doppel) {
		d.retryInterrupted = true // TODO: implement
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
