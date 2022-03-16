package doppel

import (
	"time"

	"github.com/pkg/errors"
)

// RequestError provides additional context to errors that occur during the
// request cycle.
type RequestError struct {
	error
	Target          string // the template the request attempted to retrieve
	RequestDuration time.Duration
}

// Is returns true if the Error's underlying error matches err.
func (re RequestError) Is(err error) bool {
	return re.Error() == err.Error()
}

// ErrDoppelShutdown is used in response to requests to a Doppel
// with an closed cache.
var ErrDoppelShutdown = errors.New("can't send request to stopped cache")

// ErrSchematicNotFound is used when a named TemplateSchematic isn't present
// in the Doppel's CacheSchematic.
var ErrSchematicNotFound = errors.New("requested *TemplateSchematic not found")

// ErrNotInitialized is used when a Get request is made to the
// global cache before Initialize is called.
var ErrNotInitialized = errors.New("Get was called before initializing the global cache")

// ErrAlreadyInitialized is used when the user attempts to
// call Initialize when the global cache is already running.
var ErrAlreadyInitialized = errors.New("the global cache is already running")
