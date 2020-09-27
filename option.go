package doppel

// Option are used to decorate new Doppels, e.g. adding template
// expiry or memory limits.
type Option func(*Doppel) *Doppel // TODO: Ensure custom Options can't affect unexported fields.

// TODO: Implement stale template expiry.
// func WithExpiry(expireAfter time.Duration) Option {

// }

// TODO: Implement heartbeat.
// func WithHeartbeat(pulseRate time.Duration) Option {

// }

// TODO - stretch: Implement memory limit.
// func WithMemoryLimit(limitInMB uint64) Option {

// }

// TODO: Implement request timeout.
// func WithRequestTimeout(timeout time.Duration) Option {

// }
