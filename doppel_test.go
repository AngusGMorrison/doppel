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

const gracePeriod = 100 * time.Millisecond

func TestNew(t *testing.T) {
	t.Run("CacheSchematic operations", func(t *testing.T) {
		t.Run("returns an error if schematic is cyclic", func(t *testing.T) {
			cyclicSchematic := schematic.Clone()
			cyclicSchematic["commonNav"].BaseTmplName = "withBody1"
			d, err := New(cyclicSchematic)
			if err == nil {
				t.Errorf("failed to report cycle in schematic")
			}
			if d != nil {
				t.Errorf("got *Doppel %+v, want nil", d)
				d.Shutdown(gracePeriod)
			}
		})

		t.Run("clones provided schematic before use", func(t *testing.T) {
			testSchematic := schematic.Clone()
			d, err := New(testSchematic)
			if err != nil {
				t.Fatal(err)
			}
			d.Shutdown(gracePeriod) // ensures a schematic data race is impossible

			d.schematic["base"] = nil
			if testSchematic["base"] == nil {
				t.Error("schematic was not cloned")
			}
		})
	})

	t.Run("calls functional options", func(t *testing.T) {
		testCases := []int{0, 1, 10}
		for _, optCount := range testCases {
			var optsCalled int
			opt := func(*Doppel) {
				optsCalled++
			}

			optArgs := make([]CacheOption, optCount)
			for i := range optArgs {
				optArgs[i] = opt
			}

			d, err := New(schematic, optArgs...)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Shutdown(gracePeriod)

			if optsCalled != optCount {
				t.Errorf("%d options were called, want %d", optsCalled, optCount)
			}
		}
	})

	t.Run("returned *Doppel", func(t *testing.T) {
		t.Run("has a live cache", func(t *testing.T) {
			d, err := New(schematic)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()

			d.Get(context.Background(), "anything")
			select {
			case <-d.heartbeat:
			case <-time.After(1 * time.Second):
				t.Errorf("failed to receive heartbeat before timeout")
			}
		})

		t.Run("accepts requests", func(t *testing.T) {
			d, err := New(schematic)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Shutdown(gracePeriod)

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
			d, err := New(schematic)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Shutdown(gracePeriod)

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
		log := &testLogger{out: &bytes.Buffer{}}
		d, err := New(schematic, WithLogger(log))
		if err != nil {
			t.Fatal(err)
		}
		defer d.Close()

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

		d, err := New(testSchematic)
		if err != nil {
			t.Fatal(err)
		}
		defer d.Shutdown(gracePeriod)

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
		d, err := New(schematic)
		if err != nil {
			t.Fatal(err)
		}
		defer d.Shutdown(gracePeriod)

		target := "withBody1"
		_, err = d.Get(context.Background(), target) // prime the cache

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
		d, err := New(schematic)
		if err != nil {
			t.Fatal(err)
		}
		defer d.Close()

		errStream := make(chan error)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		target := "base"
		go func() {
			_, err := d.Get(ctx, target)
			errStream <- err
		}()

		select {
		case <-d.Heartbeat(): // cancel after work has started
			cancel()
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
		testSchematic := schematic.Clone()
		testSchematic[target] = &TemplateSchematic{"", []string{"missing"}}
		log := &testLogger{out: &bytes.Buffer{}}
		d, err := New(testSchematic, WithLogger(log))
		if err != nil {
			t.Fatal(err)
		}
		defer d.Close()

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
		d, err := New(schematic)
		if err != nil {
			t.Fatal(err)
		}
		defer d.Close()

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

func TestShutdown(t *testing.T) {
	t.Run("waits until gracePeriod elapses to close requestStream", func(t *testing.T) {
		d, err := New(schematic)
		if err != nil {
			t.Fatal(err)
		}
		d.Shutdown(500 * time.Millisecond)

		hb := d.Heartbeat()
		select {
		case <-hb:
			t.Errorf("heartbeat was closed before graceful shutdown period elapsed")
		case <-time.After(450 * time.Millisecond):
		}

		select {
		case <-hb:
		case <-time.After(100 * time.Millisecond):
			t.Errorf("heartbeat failed to close")
		}
	})

	t.Run("immediately prevents Get accepting new requests", func(t *testing.T) {
		d, err := New(schematic)
		if err != nil {
			t.Fatal(err)
		}
		d.Shutdown(gracePeriod)

		tmpl, err := d.Get(context.Background(), "base")
		if tmpl != nil {
			t.Error("Doppel accepted and completed new request after shutdown")
		}
		if err != ErrDoppelShutdown {
			t.Errorf("got err %v, want ErrDoppelClosed", err)
		}
	})

	t.Run("is safe to call twice", func(t *testing.T) {
		defer func() {
			if p := recover(); p != nil {
				t.Errorf("call to d.Shutdown panicked")
			}
		}()

		d, err := New(schematic)
		if err != nil {
			t.Fatal(err)
		}
		d.Shutdown(gracePeriod)
		d.Shutdown(gracePeriod)
	})

	t.Run("is safe to call in conjunction with Close", func(t *testing.T) {
		defer func() {
			if p := recover(); p != nil {
				t.Errorf("call to d.Shutdown panicked")
			}
		}()

		d, err := New(schematic)
		if err != nil {
			t.Fatal(err)
		}
		d.Close()
		d.Shutdown(gracePeriod)
	})
}

func TestClose(t *testing.T) {
	t.Run("immediately closes d.requestStream", func(t *testing.T) {
		d, err := New(schematic)
		if err != nil {
			t.Fatal(err)
		}
		d.Close()

		select {
		case <-d.requestStream:
		default:
			t.Errorf("heartbeat failed to close")
		}

		if _, err := d.Get(context.Background(), "base"); err != ErrDoppelShutdown {
			t.Errorf("got %v, want ErrDoppelShutdown", err)
		}
	})

	t.Run("is safe to call twice", func(t *testing.T) {
		defer func() {
			if p := recover(); p != nil {
				t.Errorf("call to d.Close panicked")
			}
		}()

		d, err := New(schematic)
		if err != nil {
			t.Fatal(err)
		}
		d.Close()
		d.Close()
	})

	t.Run("is safe to call in conjunction with Shutdown", func(t *testing.T) {
		defer func() {
			if p := recover(); p != nil {
				t.Errorf("call to d.Close panicked")
			}
		}()

		d, err := New(schematic)
		if err != nil {
			t.Fatal(err)
		}
		d.Shutdown(gracePeriod)
		d.Close()
	})
}

// Run StressTest with the -race flag to ensure no race conditions
// develop under load.
func Test_StressTest(t *testing.T) {
	type testResult struct {
		target string
		err    error
	}

	d, err := New(schematic, WithRetryTimeouts())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Shutdown(gracePeriod)

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
