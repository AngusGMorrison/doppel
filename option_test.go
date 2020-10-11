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

// func TestShouldRetry(t *testing.T) {
// 	t.Run("returns the expected output for each input", func(t *testing.T) {
// 		retryDoppel, err := New(schematic, WithRetryTimeouts())
// 		if err != nil {
// 			t.Fatal(err)
// 		}
// 		defer retryDoppel.Shutdown(gracePeriod)

// 		noRetryDoppel, err := New(schematic)
// 		if err != nil {
// 			t.Fatal(err)
// 		}
// 		defer noRetryDoppel.Shutdown(gracePeriod)

// 		closed := make(chan struct{})
// 		close(closed)

// 		// Although non-exhaustive, these tests give reasonable assurance that
// 		// shouldRetry as internally they are ORed together.
// 		testCases := []struct {
// 			d    *Doppel
// 			ce   *cacheEntry
// 			req  *request
// 			want bool
// 		}{
// 			{
// 				d:    noRetryDoppel,
// 				ce:   &cacheEntry{closed, nil, nil},
// 				req:  &request{refreshCache: false, ctx: context.Background()},
// 				want: false,
// 			},
// 			{
// 				d:    retryDoppel,
// 				ce:   &cacheEntry{closed, nil, nil},
// 				req:  &request{refreshCache: false, ctx: context.Background()},
// 				want: false,
// 			},
// 			{
// 				d:    retryDoppel,
// 				ce:   &cacheEntry{closed, nil, context.DeadlineExceeded},
// 				req:  &request{refreshCache: false, ctx: context.Background()},
// 				want: true,
// 			},
// 			{
// 				d:    noRetryDoppel,
// 				ce:   &cacheEntry{closed, nil, context.Canceled},
// 				req:  &request{refreshCache: false, ctx: context.Background()},
// 				want: true,
// 			},
// 			{
// 				d:    noRetryDoppel,
// 				ce:   &cacheEntry{closed, nil, nil},
// 				req:  &request{refreshCache: true, ctx: context.Background()},
// 				want: true,
// 			},
// 		}

// 		for _, tc := range testCases {
// 			t.Run(fmt.Sprintf("retryTimeouts=%t, cachedErr=%q, refreshCache=%t",
// 				tc.d.retryTimeouts, tc.ce.err, tc.req.refreshCache),
// 				func(t *testing.T) {
// 					if got := tc.d.shouldRetry(tc.ce, tc.req); got != tc.want {
// 						t.Errorf("got %t, want %t", got, tc.want)
// 					}
// 				})
// 		}
// 	})

// 	t.Run("blocks until cacheEntry is ready or request is interrupted", func(t *testing.T) {
// 		d, err := New(schematic)
// 		if err != nil {
// 			t.Fatal(err)
// 		}
// 		defer d.Shutdown(gracePeriod)

// 		t.Run("when cache entry becomes ready", func(t *testing.T) {
// 			resultStream := make(chan bool)
// 			ce := &cacheEntry{ready: make(chan struct{})}
// 			req := &request{refreshCache: false, ctx: context.Background()}
// 			go func() {
// 				resultStream <- d.shouldRetry(ce, req)
// 			}()

// 			select {
// 			case <-resultStream:
// 				t.Error("got result before ready channel was closed")
// 				close(ce.ready)
// 			case <-time.After(500 * time.Millisecond):
// 				close(ce.ready)
// 				select {
// 				case <-resultStream:
// 				case <-time.After(500 * time.Millisecond):
// 					t.Error("timed out before receiving result after closing ready chan")
// 				}
// 			}
// 		})

// 		t.Run("when request is interrupted", func(t *testing.T) {
// 			ce := &cacheEntry{ready: make(chan struct{})}
// 			ctx, cancel := context.WithTimeout(context.Background(), 0*time.Nanosecond)
// 			defer cancel()
// 			req := &request{ctx: ctx}
// 			resultStream := make(chan bool)

// 			go func() {
// 				resultStream <- d.shouldRetry(ce, req)
// 			}()

// 			select {
// 			case res := <-resultStream:
// 				if res != false {
// 					t.Errorf("got %t, want false", res)
// 				}
// 			case <-time.After(500 * time.Millisecond):
// 				t.Fatalf("failed to unblock before timeout")
// 				close(ce.ready)
// 			}
// 		})
// 	})
// }

func TestWithExpiry(t *testing.T) {
	// TODO
}
