package adb

import (
	"context"
	"strings"
	"testing"
)

// nightFake answers `cmd uimode night` (no value) with out and records every call.
type nightFake struct {
	out   string
	calls []string
}

func (f *nightFake) runner() Runner {
	return func(_ context.Context, args ...string) ([]byte, error) {
		j := strings.Join(args, " ")
		f.calls = append(f.calls, j)
		if j == "shell cmd uimode night" {
			return []byte(f.out), nil
		}
		return nil, nil
	}
}

func (f *nightFake) last() string { return f.calls[len(f.calls)-1] }

func TestSetDarkModeRecordsPriorAndRestoresOnShutdown(t *testing.T) {
	resetOverrides(t)
	f := &nightFake{out: "Night mode: no\n"}
	newClient = func(serial string) *Client { return &Client{Serial: serial, run: f.runner()} }
	if err := newClient("R58").SetDarkMode(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if got := Overrides(); len(got) != 1 || got[0].Kind != OverrideDarkMode {
		t.Fatalf("not recorded: %v", got)
	}
	if p, _ := overrideState("R58", OverrideDarkMode); p != "no" {
		t.Errorf("prior = %q, want no", p)
	}
	if !strings.Contains(doctorOverrides(), "set_dark_mode") {
		t.Errorf("doctor does not show an undo hint: %q", doctorOverrides())
	}
	if errs := RestoreOverrides(context.Background()); len(errs) != 0 {
		t.Fatal(errs)
	}
	if f.last() != "shell cmd uimode night no" {
		t.Errorf("last call = %q, want prior put back", f.last())
	}
	if len(Overrides()) != 0 {
		t.Errorf("not cleared: %v", Overrides())
	}
}

func TestSetDarkModeUnknownPriorIsNotRestored(t *testing.T) {
	resetOverrides(t)
	f := &nightFake{out: "Unknown command: night\nx; reboot"}
	newClient = func(serial string) *Client { return &Client{Serial: serial, run: f.runner()} }
	if err := newClient("R58").SetDarkMode(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if len(Overrides()) != 1 {
		t.Fatalf("override should still be recorded: %v", Overrides())
	}
	f.calls = nil
	if errs := RestoreOverrides(context.Background()); len(errs) != 0 {
		t.Fatal(errs)
	}
	if len(f.calls) != 0 {
		t.Errorf("unknown prior must not be restored: %v", f.calls)
	}
	if len(Overrides()) != 1 {
		t.Errorf("entry should stay recorded: %v", Overrides())
	}
}

func TestSetDarkModeToggleBackToPriorClears(t *testing.T) {
	resetOverrides(t)
	f := &nightFake{out: "Night mode: no"}
	c := &Client{Serial: "R58", run: f.runner()}
	ctx := context.Background()
	if err := c.SetDarkMode(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := c.SetDarkMode(ctx, false); err != nil {
		t.Fatal(err)
	}
	if f.last() != "shell cmd uimode night no" {
		t.Errorf("last call = %q", f.last())
	}
	if len(Overrides()) != 0 {
		t.Errorf("toggling back to the prior should clear: %v", Overrides())
	}
}

func TestSetDarkModeKeepsFirstPriorAcrossRepeat(t *testing.T) {
	resetOverrides(t)
	f := &nightFake{out: "Night mode: auto"}
	c := &Client{Serial: "R58", run: f.runner()}
	ctx := context.Background()
	_ = c.SetDarkMode(ctx, true)
	f.out = "Night mode: yes" // our own change must not become the prior
	_ = c.SetDarkMode(ctx, true)
	if p, _ := overrideState("R58", OverrideDarkMode); p != "auto" {
		t.Errorf("prior = %q, want auto", p)
	}
	if len(Overrides()) != 1 {
		t.Errorf("auto prior cannot be toggled back to: %v", Overrides())
	}
}
