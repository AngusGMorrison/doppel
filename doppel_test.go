package doppel

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"text/template"
)

var (
	cwd, _   = os.Getwd()
	fixtures = filepath.Join(cwd, "test_fixtures")
	basepath = filepath.Join(fixtures, "base.gohtml")
	navpath  = filepath.Join(fixtures, "nav.gohtml")
	body1    = filepath.Join(fixtures, "body_1.gohtml")
	body2    = filepath.Join(fixtures, "body_2.gohtml")
)

var schematic = CacheSchematic{
	"base":      {"", []string{basepath}},
	"commonNav": {"base", []string{navpath}},
	"withBody1": {"commonNav", []string{body1}},
	"withBody2": {"commonNav", []string{body2}},
}

func TestHeartbeat(t *testing.T) {
	t.Run("returns a channel that receives a signal on each new request cycle", func(t *testing.T) {
		wantHeartbeats := 4
		d := New(schematic)
		d.heartbeat = make(chan struct{}, wantHeartbeats)

		for i := 0; i < wantHeartbeats-1; i++ {
			d.Get("base")
		}

		hb := d.Heartbeat()
		var gotHeartbeats int
	loop:
		for {
			select {
			case <-hb:
				gotHeartbeats++
			default:
				break loop
			}
		}

		if gotHeartbeats != wantHeartbeats {
			t.Errorf("got %d cycles, want %d\n", gotHeartbeats, wantHeartbeats)
		}
	})
}

func TestGet(t *testing.T) {
	testCases := []struct{ schematicName, fileName string }{
		{"withBody1", "body_1"},
		{"withBody2", "body_2"},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("composes and returns %s correctly", tc.schematicName), func(t *testing.T) {
			done := make(chan struct{})
			defer close(done)
			Initialize(done, schematic)

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
}
