package doppel

import (
	"bytes"
	"fmt"
	"html/template"
	"path/filepath"
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
	testCases := []struct{ schematicName, fileName string }{
		{"withBody1", "body_1"},
		{"withBody2", "body_2"},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("composes and returns %s", tc.schematicName), func(t *testing.T) {
			done := make(chan struct{})
			defer close(done)
			err := Initialize(done, schematic)
			if err != nil {
				t.Fatal(err)
			}

			tmpl, err := Get(tc.schematicName)
			if err != nil {
				t.Fatal(err)
			}

			var got bytes.Buffer
			err = tmpl.Execute(&got, nil)
			if err != nil {
				t.Fatal(err)
			}

			wantTmpl, err := template.ParseFiles(basepath, navpath,
				filepath.Join(fixtures, tc.fileName+".gohtml"))
			if err != nil {
				t.Fatal(err)
			}

			var want bytes.Buffer
			err = wantTmpl.Execute(&want, nil)
			if err != nil {
				t.Fatal(err)
			}

			if gotStr, wantStr := got.String(), want.String(); gotStr != wantStr {
				t.Fatalf("got %v, want %v\n", gotStr, wantStr)
			}
		})
	}

	t.Run("returns an error if called before Initialize", func(t *testing.T) {
		target := "base"
		_, err := Get(target)
		if err != ErrNotInitialized {
			t.Errorf("got %v, want ErrNotInitialized", err)
		}
	})
}
