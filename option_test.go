// option_test contains tests for functional options that are ameable to unit
// testing. Options that require greater integration with the package as a whole
// are excercised throughout doppel_test and cache_operatoins_test.
package doppel

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
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
	fmt.Fprintf(tl.out, msg+"\n", data...)
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
		d.Get(context.Background(), "withBody1")

		if gotLogs := l.String(); gotLogs == "" {
			t.Error("failed to log operation, got empty string")
		}
	})
}

func TestWithGlobalTimeout(t *testing.T) {
	t.Run("Get returns context.DeadlineExceeded when timeout expires", func(t *testing.T) {
		globalTimeout := 1 * time.Nanosecond
		d, err := New(schematic, WithGlobalTimeout(globalTimeout))
		if err != nil {
			t.Fatal(err)
		}
		defer d.Close()

		_, err = d.Get(context.Background(), "base")
		if err != context.DeadlineExceeded {
			t.Errorf("want context.DeadlineExceeded, got: %v", err)
		}
	})

	t.Run("Get times out after the shortest of global timeout and request timeout", func(t *testing.T) {
		globalTimeout := 1 * time.Millisecond
		d, err := New(schematic, WithGlobalTimeout(globalTimeout))
		if err != nil {
			t.Fatal(err)
		}
		defer d.Shutdown(gracePeriod)

		reqTimeout := 1 * time.Nanosecond
		errStream := make(chan error)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), reqTimeout)
			defer cancel()
			_, err := d.Get(ctx, "withBody1")
			errStream <- err
		}()

		select {
		case err := <-errStream:
			if err != context.DeadlineExceeded {
				t.Errorf("request timeout expired with error \"%v\"; want context.DeadlineExceeded",
					err)
			}
		case <-time.After(globalTimeout):
			t.Errorf("global timeout expired before Get returned; want request timeout to expire first")
		}
	})
}

func TestWithExpiry(t *testing.T) {
	// TODO
}
