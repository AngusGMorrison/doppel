package doppel

type logger interface {
	Printf(fmt string, args ...interface{})
}

// defaultLog provides a no-op logger to avoid a series of nil checks throughout
// the cache's work loop.
type defaultLog struct{}

func (d *defaultLog) Printf(format string, args ...interface{}) {
	// No-op.
}
