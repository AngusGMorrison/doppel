package doppel

import (
	"fmt"
	"testing"
	"time"
)

func TestInitialize(t *testing.T) {
	t.Run("assigns a new *Doppel with a live cache to defaultCache", func(t *testing.T) {
		done := make(chan struct{})
		defer close(done)
		Initialize(done, schematic)
		if defaultCache == nil {
			t.Fatal("failed to assign defaultCache")
		}
		target := "base"
		if _, err := Get(target); err != nil {
			t.Errorf("Get(%q) returned err: %v", target, err)
		}
	})

	t.Run("passes the done channel to the *Doppel created", func(t *testing.T) {
		done := make(chan struct{})
		Initialize(done, schematic)
		close(done)
		select {
		case <-defaultCache.done:
		case <-time.After(1 * time.Second):
			t.Errorf("defaultCache.done failed to close before the timeout expired")
		}
	})
}

func TestGet(t *testing.T) {
	t.Run(fmt.Sprintf("returns the requested template"), func(t *testing.T) {
		target := "withBody1"
		done := make(chan struct{})
		defer close(done)

		err := Initialize(done, schematic)
		if err != nil {
			t.Fatal(err)
		}
		gotTemplate, err := Get(target)
		if err != nil {
			t.Fatal(err)
		}

		// Cursory check to ensure template contains the expected
		// subtemplates. TestDoppelGet exercises the full logic.
		for _, name := range []string{"base.gohtml", "nav.gohtml", "body_1.gohtml"} {
			if subTmpl := gotTemplate.Lookup(name); subTmpl == nil {
				t.Errorf("received template does not contain subtemplate %s", name)
			}
		}
	})

	t.Run("returns an error if called before Initialize", func(t *testing.T) {
		defaultCache = nil
		_, err := Get("base")
		if err != ErrNotInitialized {
			t.Errorf("got err %q, want ErrNotInitialized", err)
		}
	})
}
