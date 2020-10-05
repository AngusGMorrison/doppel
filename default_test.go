package doppel

import (
	"fmt"
	"testing"
)

func TestInitialize(t *testing.T) {
	t.Run("assigns a new *Doppel with a live cache to defaultCache", func(t *testing.T) {
		err := Initialize(schematic)
		if err != nil {
			t.Fatal(err)
		}
		defer Close()
		if defaultCache == nil {
			t.Fatal("failed to assign defaultCache")
		}
	})

	t.Run("returns an error if the default cache is already initialized", func(t *testing.T) {
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

func TestGet(t *testing.T) {
	t.Run(fmt.Sprintf("returns the requested template"), func(t *testing.T) {
		target := "withBody1"
		err := Initialize(schematic)
		if err != nil {
			t.Fatal(err)
		}
		defer Close()

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
