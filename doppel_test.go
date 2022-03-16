package doppel

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	cwd, _    = os.Getwd()
	fixtures  = filepath.Join(cwd, "test_fixtures")
	basepath  = filepath.Join(fixtures, "base.gohtml")
	navpath   = filepath.Join(fixtures, "nav.gohtml")
	body1Path = filepath.Join(fixtures, "body_1.gohtml")
	body2Path = filepath.Join(fixtures, "body_2.gohtml")
)

var schematic = CacheSchematic{
	"base":      {"", []string{basepath}},
	"commonNav": {"base", []string{navpath}},
	"withBody1": {"commonNav", []string{body1Path}},
	"withBody2": {"commonNav", []string{body2Path}},
}

func TestNew(t *testing.T) {
	t.Run("CacheSchematic operations", func(t *testing.T) {
		t.Run("returns an error if schematic is cyclic", func(t *testing.T) {
			cyclicSchematic := schematic.Clone()
			cyclicSchematic["commonNav"].BaseTmplName = "withBody1"

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			d, err := New(ctx, cyclicSchematic)
			if err == nil {
				t.Errorf("failed to report cycle in schematic")
			}
			if d != nil {
				t.Errorf("got *Doppel %+v, want nil", d)
			}
		})

		t.Run("clones provided schematic before use", func(t *testing.T) {
			testSchematic := schematic.Clone()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			d, err := New(ctx, testSchematic)
			if err != nil {
				t.Fatal(err)
			}
			cancel() // stop the cache to ensure schema data races don't muddy test results

			d.schematic["base"] = nil
			if testSchematic["base"] == nil {
				t.Error("schematic was not cloned")
			}
		})
	})

	t.Run("calls functional options", func(t *testing.T) {
		for _, optCount := range []int{0, 1, 10} {
			var optsCalled int
			opt := func(*Doppel) {
				optsCalled++
			}

			optArgs := make([]CacheOption, optCount)
			for i := range optArgs {
				optArgs[i] = opt
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			_, err := New(ctx, schematic, optArgs...)
			if err != nil {
				t.Fatal(err)
			}

			if optsCalled != optCount {
				t.Errorf("%d options were called, want %d", optsCalled, optCount)
			}
		}
	})

	t.Run("returned *Doppel", func(t *testing.T) {
		t.Run("has a live cache", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			d, err := New(ctx, schematic)
			if err != nil {
				t.Fatal(err)
			}

			d.Get(context.Background(), "anything")
			select {
			case <-d.heartbeat:
			case <-time.After(1 * time.Second):
				t.Errorf("failed to receive heartbeat before timeout")
			}
		})

		t.Run("accepts requests", func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			d, err := New(ctx, schematic)
			if err != nil {
				t.Fatal(err)
			}

			req := &request{
				name:         "base",
				resultStream: make(chan<- *result, 1),
				ctx:          context.Background(),
			}

			select {
			case d.requestStream <- req:
			case <-time.After(1 * time.Second):
				t.Error("request timed out before being accepted")
			}
		})
	})
}

func TestGet(t *testing.T) {
	testCases := []struct {
		schematicName string
		files         []string
	}{
		{"withBody1", []string{basepath, navpath, body1Path}},
		{"withBody2", []string{basepath, navpath, body2Path}},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("composes and returns %s", tc.schematicName), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			d, err := New(ctx, schematic)
			if err != nil {
				t.Fatal(err)
			}

			tmpl, err := d.Get(context.Background(), tc.schematicName)
			if err != nil {
				t.Fatal(err)
			}
			var got bytes.Buffer
			err = tmpl.Execute(&got, nil)
			if err != nil {
				t.Fatal(err)
			}

			wantTmpl, err := template.ParseFiles(tc.files...)
			if err != nil {
				t.Fatal(err)
			}
			var want bytes.Buffer
			err = wantTmpl.Execute(&want, nil)
			if err != nil {
				t.Fatal(err)
			}

			if gotStr, wantStr := got.String(), want.String(); gotStr != wantStr {
				t.Fatalf("got %v, want %v\n", gotStr, wantStr) // TODO: Display as diff
			}
		})
	}

	t.Run("caches parsed templates", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		log := &testLogger{out: &bytes.Buffer{}}
		d, err := New(ctx, schematic, WithLogger(log))
		if err != nil {
			t.Fatal(err)
		}

		target := "withBody1"
		_, err = d.Get(context.Background(), target) // prime cache
		if err != nil {
			t.Fatal(err)
		}
		log.mu.Lock()
		log.out = &bytes.Buffer{}
		log.mu.Unlock()

		_, err = d.Get(context.Background(), target) // get cached template
		if err != nil {
			t.Fatal(err)
		}

		logged := log.String()
		if logged == "" {
			t.Fatalf("failed to record cache logs")
		}
		msg := fmt.Sprintf(logParsingTemplate, target)
		if strings.Contains(logged, msg) {
			t.Errorf("template was parsed, not cached")
		}
	})

	t.Run("returns an error if any constituent TemplateSchematic is not found", func(t *testing.T) {
		testSchematic := schematic.Clone()
		testSchematic["incomplete"] = &TemplateSchematic{
			BaseTmplName: "missing",
			Filepaths:    []string{},
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		d, err := New(ctx, testSchematic)
		if err != nil {
			t.Fatal(err)
		}

		for _, name := range []string{"incomplete", "missing"} {
			tmpl, err := d.Get(context.Background(), name)
			if tmpl != nil {
				t.Errorf("want d.Get(%q) to return nil template, got %+v", name, tmpl)
			}
			if err == nil {
				t.Errorf("d.Get(%q) failed to return an error", name)
			}
		}
	})

	t.Run("returns context.DeadlineExceeded if the request times out", func(t *testing.T) {
		// Response time is non-deterministic, so excersise the full
		// range of preemption points via random testing.
		type testResult struct {
			target  string
			timeout time.Duration
			err     error
		}

		resultStream := make(chan *testResult)
		rng := rand.New(rand.NewSource(time.Now().UnixNano()))

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		d, err := New(ctx, schematic)
		if err != nil {
			t.Fatal(err)
		}

		target := "withBody1"
		_, err = d.Get(context.Background(), target) // prime the cache
		if err != nil {
			t.Fatalf("failed to prime the cache with Get(ctx, %q); got error %v", target, err)
		}

		var wg sync.WaitGroup
		count := 50
		wg.Add(count)
		fmt.Println("running pseudorandom timeout preemption tests...")
		for i := 0; i < count; i++ {
			timeout := time.Duration(rng.Intn(1e4)) * time.Nanosecond

			go func(target string, timeout time.Duration) {
				result := &testResult{target: target, timeout: timeout}
				ctx, cancel := context.WithTimeout(context.Background(), timeout)
				defer cancel()

				_, err := d.Get(ctx, target)
				result.err = err
				resultStream <- result
				wg.Done()
			}(target, timeout)
		}

		go func() {
			wg.Wait()
			close(resultStream)
		}()

		for res := range resultStream {
			fmt.Printf("\tcalling d.Get(%q) with timeout %d ns...\n", res.target, res.timeout)
			switch {
			case res.err == nil:
				fmt.Println("\t✔ returned template before timeout")
			case errors.Is(res.err, context.DeadlineExceeded):
				fmt.Println("\t✔ timed out with context.DeadlineExceeded")
			default:
				t.Fatalf(
					"d.Get(%q) with timeout %d ns: got error %q, want context.DeadlineExceeded",
					res.target, res.timeout, res.err,
				)
			}
		}
	})

	t.Run("returns context.Canceled if the request is canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		d, err := New(ctx, schematic)
		if err != nil {
			t.Fatal(err)
		}

		errStream := make(chan error)
		reqCtx, reqCancel := context.WithCancel(context.Background())
		defer reqCancel()
		target := "base"
		go func() {
			_, err := d.Get(reqCtx, target)
			errStream <- err
		}()

		select {
		case <-d.Heartbeat(): // cancel after work has started
			reqCancel()
		case <-errStream:
			t.Fatalf("request completed before cancellation")
		}

		err = <-errStream
		if err != context.Canceled {
			t.Errorf("want error context.Canceled, got: %v", err)
		}
	})

	t.Run("caches errored results", func(t *testing.T) {
		target := "error"

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		testSchematic := schematic.Clone()
		testSchematic[target] = &TemplateSchematic{"", []string{"missing"}}
		log := &testLogger{out: &bytes.Buffer{}}
		d, err := New(ctx, testSchematic, WithLogger(log))
		if err != nil {
			t.Fatal(err)
		}

		_, err = d.Get(context.Background(), target)
		if err == nil {
			t.Fatalf("d.Get(%q) failed to return an error", target)
		}

		logged := log.String()
		wantEntry := fmt.Sprintf(logDeliveringCachedError, target)
		if !strings.Contains(logged, wantEntry) {
			t.Errorf("d.Get(%q): error was not cached", target)
		}
	})
}

func TestIsCyclic(t *testing.T) {
	testCycle := func(start, end string, t *testing.T) {
		cyclicSchematic := schematic.Clone()
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

func TestHeartbeat(t *testing.T) {
	t.Run("returns a channel that receives a signal on each new request cycle", func(t *testing.T) {
		const timeout = 1
		const wantHeartbeats = 4

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		d, err := New(ctx, schematic)
		if err != nil {
			t.Fatal(err)
		}

		hb := d.Heartbeat()
		var gotHeartbeats int
		heartbeatOrTimeout := func() {
			select {
			case <-hb:
				gotHeartbeats++
			case <-time.After(timeout * time.Second):
				t.Fatal("timed out before receiving heartbeat")
			}
		}

		for i := 0; i < wantHeartbeats; i++ {
			d.Get(context.Background(), "base")
			heartbeatOrTimeout()
		}

		if gotHeartbeats != wantHeartbeats {
			t.Errorf("got %d heartbeats, want %d\n", gotHeartbeats, wantHeartbeats)
		}
	})
}

// Run StressTest with the -race flag to ensure no race conditions
// develop under load.
func Test_StressTest(t *testing.T) {
	type testResult struct {
		target string
		err    error
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	d, err := New(ctx, schematic, WithRetryTimeouts())
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	resultStream := make(chan *testResult)
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	keys := make([]string, 0, len(schematic))
	for k := range schematic {
		keys = append(keys, k)
	}

	for i := 0; i < 5000; i++ {
		target := keys[rng.Intn(len(keys))]
		timeout := rng.Intn(1e4)
		wg.Add(1)
		go func() {
			var err error
			if timeout%2 == 0 {
				_, err = d.Get(context.Background(), target)
			} else {
				ctx, cancel := context.WithTimeout(context.Background(),
					time.Duration(timeout)*time.Nanosecond)
				defer cancel()
				_, err = d.Get(ctx, target)
			}
			resultStream <- &testResult{target, err}
			wg.Done()
		}()
	}

	go func() {
		wg.Wait()
		close(resultStream)
	}()

	for res := range resultStream {
		if res.err != nil && !errors.Is(res.err, context.DeadlineExceeded) {
			t.Errorf("d.Get(%q) returned error %q", res.target, res.err)
		}
	}
}
