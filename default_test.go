package doppel

import (
	"bytes"
	"fmt"
	"html/template"
	"path/filepath"
	"testing"
)

func TestInitialize(t *testing.T) {
	// TODO
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
		// TODO
	})
}
