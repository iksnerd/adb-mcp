package adb

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

// uiFake scripts the dump flow: it serves `cat` from files written by
// `uiautomator dump`, so a dump that writes nothing leaves any older file.
type uiFake struct {
	mu       sync.Mutex
	calls    []string
	files    map[string]string
	dumpOut  string // stdout of `uiautomator dump`
	dumpFile string // content the dump writes ("" = writes nothing)
	wm       string
	input    string // stdout of `dumpsys input`
}

func (f *uiFake) run(_ context.Context, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	j := strings.Join(args, " ")
	f.calls = append(f.calls, j)
	if f.files == nil {
		f.files = map[string]string{}
	}
	switch {
	case len(args) >= 4 && args[1] == "rm" && args[2] == "-f":
		delete(f.files, args[3])
	case len(args) >= 4 && args[1] == "uiautomator" && args[2] == "dump":
		if f.dumpFile != "" {
			f.files[args[3]] = f.dumpFile
		}
		return []byte(f.dumpOut), nil
	case len(args) >= 3 && args[1] == "cat":
		c, ok := f.files[args[2]]
		if !ok {
			return nil, errors.New("No such file or directory")
		}
		return []byte(c), nil
	case strings.HasPrefix(j, "shell wm size"):
		return []byte(f.wm), nil
	case strings.HasPrefix(j, "shell dumpsys input"):
		return []byte(f.input), nil
	}
	return nil, nil
}

func TestDumpUsesUniquePathAndCleansUp(t *testing.T) {
	f := &uiFake{dumpFile: hierWithCancel, dumpOut: "UI hierarchy dumped to: x"}
	c := &Client{Serial: "s", run: f.run}
	paths := map[string]bool{}
	for range 2 {
		if _, err := c.dumpOnce(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	for _, call := range f.calls {
		if p, ok := strings.CutPrefix(call, "shell uiautomator dump "); ok {
			if p == "/sdcard/window_dump.xml" {
				t.Fatalf("dump still uses the shared fixed path")
			}
			paths[p] = true
		}
	}
	if len(paths) != 2 {
		t.Fatalf("want a distinct device path per call, got %v", paths)
	}
	if len(f.files) != 0 {
		t.Errorf("dump file not cleaned up: %v", f.files)
	}
}

func TestDumpExitZeroButNoFileIsError(t *testing.T) {
	// uiautomator exited 0 but wrote nothing: must not be read as a tree.
	f := &uiFake{dumpFile: "", dumpOut: "ERROR: could not get idle state."}
	c := &Client{Serial: "s", run: f.run}
	if _, err := c.dumpOnce(context.Background()); err == nil {
		t.Fatal("want an error when the dump produced no file")
	}
}

func TestDumpIgnoresPreviousFile(t *testing.T) {
	f := &uiFake{}
	f.files = map[string]string{"/sdcard/window_dump.xml": hierWithCancel}
	c := &Client{Serial: "s", run: f.run}
	if _, err := c.dumpOnce(context.Background()); err == nil {
		t.Fatal("a stale file must not satisfy a failed dump")
	}
}

func TestDumpEmptyFileIsError(t *testing.T) {
	f := &uiFake{dumpFile: "  \n", dumpOut: "UI hierarchy dumped to: x"}
	c := &Client{Serial: "s", run: f.run}
	if _, err := c.dumpOnce(context.Background()); err == nil {
		t.Fatal("empty dump file must be an error")
	}
}

func TestDisplaySizePrefersOverride(t *testing.T) {
	f := &uiFake{wm: "Physical size: 1080x2400\nOverride size: 720x1600\n"}
	c := &Client{Serial: "s", run: f.run}
	if w, h := c.displaySize(context.Background()); w != 720 || h != 1600 {
		t.Errorf("got %dx%d, want override 720x1600", w, h)
	}
	f.wm = "Physical size: 1080x2400\n"
	if w, h := c.displaySize(context.Background()); w != 1080 || h != 2400 {
		t.Errorf("got %dx%d, want physical 1080x2400", w, h)
	}
}

func TestDisplaySizeSwapsInLandscape(t *testing.T) {
	f := &uiFake{wm: "Physical size: 1080x2400\n", input: "  SurfaceOrientation: 1\n"}
	c := &Client{Serial: "s", run: f.run}
	if w, h := c.displaySize(context.Background()); w != 2400 || h != 1080 {
		t.Errorf("got %dx%d, want rotated 2400x1080", w, h)
	}
	f.input = "  SurfaceOrientation: 0\n"
	if w, h := c.displaySize(context.Background()); w != 1080 || h != 2400 {
		t.Errorf("got %dx%d, want portrait", w, h)
	}
}
