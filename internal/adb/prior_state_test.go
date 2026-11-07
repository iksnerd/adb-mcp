package adb

import (
	"context"
	"strings"
	"testing"
)

// settingsFake answers `settings get global <key>` from vals and records every
// call. Anything else succeeds with no output.
type settingsFake struct {
	vals  map[string]string
	calls []string
}

func (f *settingsFake) runner() Runner {
	return func(_ context.Context, args ...string) ([]byte, error) {
		j := strings.Join(args, " ")
		f.calls = append(f.calls, j)
		if strings.HasPrefix(j, "shell settings get global ") {
			return []byte(f.vals[strings.TrimPrefix(j, "shell settings get global ")] + "\n"), nil
		}
		return nil, nil
	}
}

func (f *settingsFake) has(sub string) bool {
	for _, c := range f.calls {
		if strings.Contains(c, sub) {
			return true
		}
	}
	return false
}

func TestStatusBarDemoRestoresPriorDemoAllowed(t *testing.T) {
	resetOverrides(t)
	f := &settingsFake{vals: map[string]string{"sysui_demo_allowed": "0"}}
	c := &Client{Serial: "R58", run: f.runner()}
	if err := c.StatusBarDemo(context.Background(), true, StatusBarOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := c.StatusBarDemo(context.Background(), false, StatusBarOptions{}); err != nil {
		t.Fatal(err)
	}
	last := f.calls[len(f.calls)-1]
	if last != "shell settings put global sysui_demo_allowed 0" {
		t.Errorf("last call = %q, want the prior value put back", last)
	}
	if len(Overrides()) != 0 {
		t.Fatalf("not cleared: %v", Overrides())
	}
}

func TestStatusBarDemoDeletesDemoAllowedThatWasUnset(t *testing.T) {
	resetOverrides(t)
	f := &settingsFake{vals: map[string]string{"sysui_demo_allowed": "null"}}
	c := &Client{Serial: "R58", run: f.runner()}
	if err := c.StatusBarDemo(context.Background(), true, StatusBarOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := c.StatusBarDemo(context.Background(), false, StatusBarOptions{}); err != nil {
		t.Fatal(err)
	}
	if !f.has("settings delete global sysui_demo_allowed") {
		t.Errorf("unset prior should be deleted: %v", f.calls)
	}
}

func TestStatusBarDemoLeavesAlreadyAllowedAlone(t *testing.T) {
	resetOverrides(t)
	f := &settingsFake{vals: map[string]string{"sysui_demo_allowed": "1"}}
	c := &Client{Serial: "R58", run: f.runner()}
	_ = c.StatusBarDemo(context.Background(), true, StatusBarOptions{})
	f.calls = nil
	_ = c.StatusBarDemo(context.Background(), false, StatusBarOptions{})
	// Exit itself puts 1 so the broadcast is allowed; nothing may change it back.
	if f.has("settings delete") || f.has("sysui_demo_allowed 0") {
		t.Errorf("prior was 1, nothing to restore: %v", f.calls)
	}
}

// Entering twice must keep the first prior, not the 1 this process wrote.
func TestStatusBarDemoKeepsFirstPriorAcrossRepeatEnter(t *testing.T) {
	resetOverrides(t)
	f := &settingsFake{vals: map[string]string{"sysui_demo_allowed": "0"}}
	c := &Client{Serial: "R58", run: f.runner()}
	_ = c.StatusBarDemo(context.Background(), true, StatusBarOptions{})
	f.vals["sysui_demo_allowed"] = "1" // what our own put left behind
	_ = c.StatusBarDemo(context.Background(), true, StatusBarOptions{})
	_ = c.StatusBarDemo(context.Background(), false, StatusBarOptions{})
	if last := f.calls[len(f.calls)-1]; last != "shell settings put global sysui_demo_allowed 0" {
		t.Errorf("last call = %q, want the original prior", last)
	}
}

func TestRestoreOverridesRestoresDemoAllowed(t *testing.T) {
	resetOverrides(t)
	f := &settingsFake{vals: map[string]string{"sysui_demo_allowed": "0"}}
	newClient = func(serial string) *Client { return &Client{Serial: serial, run: f.runner()} }
	c := newClient("R58")
	if err := c.StatusBarDemo(context.Background(), true, StatusBarOptions{}); err != nil {
		t.Fatal(err)
	}
	f.calls = nil
	if errs := RestoreOverrides(context.Background()); len(errs) != 0 {
		t.Fatal(errs)
	}
	if !f.has("command exit") || !f.has("settings put global sysui_demo_allowed 0") {
		t.Errorf("shutdown restore incomplete: %v", f.calls)
	}
}

func TestStayAwakeRestoresPriorSetting(t *testing.T) {
	resetOverrides(t)
	f := &settingsFake{vals: map[string]string{"stay_on_while_plugged_in": "2"}}
	c := &Client{Serial: "R58", run: f.runner()}
	if err := c.StayAwake(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if got := Overrides(); len(got) != 1 || got[0].Kind != OverrideStayAwake {
		t.Fatalf("not recorded: %v", got)
	}
	if err := c.StayAwake(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if last := f.calls[len(f.calls)-1]; last != "shell settings put global stay_on_while_plugged_in 2" {
		t.Errorf("last call = %q, want prior put back", last)
	}
	if len(Overrides()) != 0 {
		t.Fatalf("not cleared: %v", Overrides())
	}
}

func TestStayAwakeIgnoresUnparsablePrior(t *testing.T) {
	resetOverrides(t)
	f := &settingsFake{vals: map[string]string{"stay_on_while_plugged_in": "x; reboot"}}
	c := &Client{Serial: "R58", run: f.runner()}
	_ = c.StayAwake(context.Background(), true)
	_ = c.StayAwake(context.Background(), false)
	if f.has("reboot") {
		t.Errorf("device output reached a command: %v", f.calls)
	}
}

func TestRestoreOverridesRestoresStayAwakeOnlyWhenPriorKnown(t *testing.T) {
	resetOverrides(t)
	f := &settingsFake{vals: map[string]string{"stay_on_while_plugged_in": "0"}}
	newClient = func(serial string) *Client { return &Client{Serial: serial, run: f.runner()} }
	if err := newClient("R58").StayAwake(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	NoteOverride("R59", OverrideStayAwake) // prior unknown: left alone
	f.calls = nil
	if errs := RestoreOverrides(context.Background()); len(errs) != 0 {
		t.Fatal(errs)
	}
	if !f.has("settings put global stay_on_while_plugged_in 0") {
		t.Errorf("known prior not restored: %v", f.calls)
	}
	got := Overrides()
	if len(got) != 1 || got[0].Serial != "R59" {
		t.Errorf("want only the unknown-prior entry left, got %v", got)
	}
}
