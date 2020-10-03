package doppel

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"text/template"
	"time"
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

func TestNew(t *testing.T) {
	t.Run("CacheSchematic operations", func(t *testing.T) {
		t.Run("returns an error if schematic is cyclic", func(t *testing.T) {

		})

		t.Run("clones provided schematic before use", func(t *testing.T) {

		})
	})

	t.Run("calls functional options", func(t *testing.T) {
		testCases := []int{0, 1, 10}
		for _, optCount := range testCases {
			var optsCalled int
			opt := func(*Doppel) {
				optsCalled++
			}

			optArgs := make([]Option, optCount)
			for i := range optArgs {
				optArgs[i] = opt
			}

			d := New(schematic, optArgs...)
			defer d.Close()

			if optsCalled != optCount {
				t.Errorf("%d options were called, want %d", optsCalled, optCount)
			}
		}
	})

	t.Run("returned *Doppel", func(t *testing.T) {
		t.Run("has a live cache", func(t *testing.T) {

		})

		t.Run("accepts requests", func(t *testing.T) {

		})
	})
}

func TestHeartbeat(t *testing.T) {
	t.Run("returns a channel that receives a signal on each new request cycle", func(t *testing.T) {
		const timeout = 1
		const wantHeartbeats = 4
		var gotHeartbeats int
		d := New(schematic)
		defer d.Close()
		hb := d.Heartbeat()

		heartbeatOrTimeout := func() {
			select {
			case <-hb:
				gotHeartbeats++
			case <-time.After(timeout * time.Second):
				t.Fatal("timed out before receiving heartbeat")
			}
		}

		heartbeatOrTimeout()
		for i := 0; i < wantHeartbeats-1; i++ {
			d.Get("base")
			heartbeatOrTimeout()
		}

		if gotHeartbeats != wantHeartbeats {
			t.Errorf("got %d heartbeats, want %d\n", gotHeartbeats, wantHeartbeats)
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

func TestIsCyclic(t *testing.T) {
	testCycle := func(start, end string, t *testing.T) {
		cyclicSchematic := schematic.clone()
		cyclicSchematic[end].BaseTmplName = start

		cycle, err := IsCyclic(cyclicSchematic)
		if !cycle {
			t.Errorf("failed to detect cycle: %q -> %q", start, end)
		}
		if err == nil {
			t.Errorf("cyclic schematic failed to return an error")
		}
	}

	testCases := []struct {
		desc, start, end string
	}{
		{"detects single-node cycles", "commonNav", "commonNav"},
		{"detects two-node cycles", "withBody1", "commonNav"},
		{"detects multi-node cycles", "withBody1", "base"},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			testCycle(tc.start, tc.end, t)
		})
	}

	t.Run("returns false for acylic schematics", func(t *testing.T) {
		cycle, err := IsCyclic(schematic)
		if cycle {
			t.Error("got true, want false")
		}
		if err != nil {
			t.Error(err)
		}
	})
}
