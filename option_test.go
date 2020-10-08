package doppel

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
)

// Go's log package recycles its buffer for each log entry, causing a data
// race if the buffer is accessed externally to the logger while logging is
// ongoing. testLogger preserves its buffer for analysis and protects it with
// a mutex.
type testLogger struct {
	mu  sync.Mutex
	out *bytes.Buffer
}

func (tl *testLogger) Printf(msg string, data ...interface{}) {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	fmt.Fprintf(tl.out, msg, data...)
}

func (tl *testLogger) String() string {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	return tl.out.String()
}

func TestWithLogger(t *testing.T) {
	t.Run("logs cache operations to the logger provded", func(t *testing.T) {
		l := &testLogger{out: &bytes.Buffer{}}
		d, err := New(schematic, WithLogger(l))
		if err != nil {
			t.Fatal(err)
		}
		defer d.Shutdown(gracePeriod)
		d.Get("withBody1")

		if gotLogs := l.String(); gotLogs == "" {
			t.Error("failed to log operation, got empty string")
		}
		fmt.Println(l.String())
	})

	// TODO: Test for specific logging events.
}

func TestWithGlobalTimemout(t *testing.T) {
	// TODO
}

func TestWithTimeout(t *testing.T) {
	// TODO
	// Does it make sense to reattempt timed-out requests, when all
	// requests will have the same timeout? Do requests need
	// functional options of their own?
}
