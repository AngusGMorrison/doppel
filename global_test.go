package doppel

import (
	"context"
	"testing"
)

func TestInitialize(t *testing.T) {
	t.Run("assigns a new *Doppel with a live cache to globalCache", func(t *testing.T) {
		err := Initialize(schematic)
		if err != nil {
			t.Fatal(err)
		}
		defer Close()
		if globalCache == nil {
			t.Fatal("failed to assign globalCache")
		}
	})

	t.Run("returns an error if the global cache is already initialized", func(t *testing.T) {
		err := Initialize(schematic)
		if err != nil {
			t.Fatal(err)
		}
		defer Close()
		err = Initialize(schematic)
		if err != ErrAlreadyInitialized {
			t.Errorf("got error %q, want ErrAlreadyInitialized", err)
		}
	})
}

func TestGlobalGet(t *testing.T) {
	t.Run("returns the requested template", func(t *testing.T) {
		target := "withBody1"
		err := Initialize(schematic)
		if err != nil {
			t.Fatal(err)
		}
		defer Close()

		gotTemplate, err := Get(context.Background(), target)
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
		globalCache = nil
		_, err := Get(context.Background(), "base")
		if err != ErrNotInitialized {
			t.Errorf("got err %q, want ErrNotInitialized", err)
		}
	})
}

func TestGlobalShutdown(t *testing.T) {
	err := Initialize(schematic)
	if err != nil {
		t.Fatal(err)
	}
	Shutdown(gracePeriod)

	// Ensure that the underlying globalCache.Shutdown has been called, which
	// is tested separately.
	_, err = Get(context.Background(), "base")
	if err != ErrDoppelShutdown {
		t.Errorf("want ErrDoppelClosed, got %v", err)
	}
}
