package doppel

import (
	"context"
	"testing"

	"github.com/pkg/errors"
)

func TestSignalStatus(t *testing.T) {
	t.Run("returns the expected output for each input", func(t *testing.T) {
		testCases := []struct {
			err             error
			retryTimeouts   bool
			wantRetrySignal bool
			wantReadySignal bool
		}{
			{context.Canceled, false, true, false},
			{context.Canceled, true, true, false},
			{context.DeadlineExceeded, false, false, true},
			{context.DeadlineExceeded, true, true, false},
			{nil, false, false, true},
			{nil, true, false, true},
			{errors.New("some error"), false, false, true},
			{errors.New("some error"), true, false, true},
		}

		for _, tc := range testCases {
			ce := &cacheEntry{
				err:   tc.err,
				retry: make(chan struct{}),
				ready: make(chan struct{}),
			}
			ce.signalStatus(tc.retryTimeouts)

			select {
			case <-ce.retry:
				if !tc.wantRetrySignal {
					t.Errorf("err=%v, retryTimeouts=%t: received unwanted retry signal",
						tc.err, tc.retryTimeouts)
				}
			default:
			}

			select {
			case <-ce.ready:
				if !tc.wantReadySignal {
					t.Errorf("err=%v, retryTimeouts=%t: received unwanted ready signal",
						tc.err, tc.retryTimeouts)
				}
			default:
			}
		}
	})
}
