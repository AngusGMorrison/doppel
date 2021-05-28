package doppel

import (
	"context"
	"testing"

	"github.com/pkg/errors"
)

func TestInitialize(t *testing.T) {
	t.Run("assigns a new *Doppel with a live cache to globalCache", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		err := Initialize(ctx, schematic)
		if err != nil {
			t.Fatal(err)
		}

		if globalCache == nil {
			t.Fatal("failed to assign globalCache")
		}
	})

	t.Run("returns an error if the global cache is already initialized", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		err := Initialize(ctx, schematic)
		if err != nil {
			t.Fatal(err)
		}

		err = Initialize(ctx, schematic)
		if errors.Cause(err) != ErrAlreadyInitialized {
			t.Errorf("got error %q, want ErrAlreadyInitialized", err)
		}
	})
}

func TestGlobalGet(t *testing.T) {
	t.Run("returns the requested template", func(t *testing.T) {
		target := "withBody1"

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		err := Initialize(ctx, schematic)
		if err != nil {
			t.Fatal(err)
		}

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
		if errors.Cause(err) != ErrNotInitialized {
			t.Errorf("got err %q, want ErrNotInitialized", err)
		}
	})
}
