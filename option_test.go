package doppel

import (
	"bytes"
	"log"
	"testing"
)

func TestWithLogger(t *testing.T) {
	t.Run("logs cache operations to the logger provded", func(t *testing.T) {
		var buf bytes.Buffer
		l := log.New(&buf, "", 0)
		d, err := New(schematic, WithLogger(l))
		if err != nil {
			t.Fatal(err)
		}
		defer d.Shutdown(gracePeriod)
		d.Get("withBody1")

		if gotLogs := buf.String(); gotLogs == "" {
			t.Error("failed to log operation, got empty string")
		}
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
