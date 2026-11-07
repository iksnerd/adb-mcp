package adb

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// callLog is a fake Runner that records every adb call and fails any call
// whose joined args contain failOn.
type callLog struct {
	mu     sync.Mutex
	calls  []string
	failOn string
}

func (l *callLog) runner() Runner {
	return func(ctx context.Context, args ...string) ([]byte, error) {
		j := strings.Join(args, " ")
		l.mu.Lock()
		l.calls = append(l.calls, j)
		l.mu.Unlock()
		if l.failOn != "" && strings.Contains(j, l.failOn) {
			return nil, errors.New("boom")
		}
		return nil, nil
	}
}

func (l *callLog) has(sub string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, c := range l.calls {
		if strings.Contains(c, sub) {
			return true
		}
	}
	return false
}

func resetOverrides(t *testing.T) {
	t.Helper()
	clearAllOverrides()
	t.Cleanup(func() {
		clearAllOverrides()
		newClient = New
	})
}

func TestOverrideRegistryAddRemove(t *testing.T) {
	resetOverrides(t)
	NoteOverride("emulator-5554", OverrideBattery)
	NoteOverride("emulator-5554", OverrideBattery) // idempotent
	NoteOverride("R58", OverrideStayAwake)
	if got := Overrides(); len(got) != 2 {
		t.Fatalf("want 2 overrides, got %v", got)
	}
	ClearOverride("emulator-5554", OverrideBattery)
	got := Overrides()
	if len(got) != 1 || got[0].Serial != "R58" || got[0].Kind != OverrideStayAwake {
		t.Fatalf("after clear: %v", got)
	}
	ClearOverride("nope", OverrideBattery) // no-op
}

func TestSetBatteryRecordsAndResetClears(t *testing.T) {
	resetOverrides(t)
	l := &callLog{}
	c := &Client{Serial: "R58", run: l.runner()}
	lvl := 5
	if err := c.SetBattery(context.Background(), &lvl, nil, false); err != nil {
		t.Fatal(err)
	}
	if len(Overrides()) != 1 {
		t.Fatalf("battery not recorded: %v", Overrides())
	}
	if err := c.SetBattery(context.Background(), nil, nil, true); err != nil {
		t.Fatal(err)
	}
	if len(Overrides()) != 0 {
		t.Fatalf("reset did not clear: %v", Overrides())
	}
}

func TestSetBatteryFailureNotRecorded(t *testing.T) {
	resetOverrides(t)
	l := &callLog{failOn: "battery set"}
	c := &Client{Serial: "R58", run: l.runner()}
	lvl := 5
	if err := c.SetBattery(context.Background(), &lvl, nil, false); err == nil {
		t.Fatal("want error")
	}
	if len(Overrides()) != 0 {
		t.Fatalf("failed call recorded: %v", Overrides())
	}
}

func TestLockRecordsFactOnly(t *testing.T) {
	resetOverrides(t)
	l := &callLog{}
	c := &Client{Serial: "R58", run: l.runner()}
	if err := c.SetDeviceLock(context.Background(), "pin", "123456", ""); err != nil {
		t.Fatal(err)
	}
	got := Overrides()
	if len(got) != 1 || got[0].Kind != OverrideDeviceLock {
		t.Fatalf("lock not recorded: %v", got)
	}
	if strings.Contains(doctorOverrides(), "123456") {
		t.Fatal("credential leaked into report")
	}
	if err := c.ClearDeviceLock(context.Background(), "123456"); err != nil {
		t.Fatal(err)
	}
	if len(Overrides()) != 0 {
		t.Fatalf("clear did not remove: %v", Overrides())
	}
}

func TestStatusBarDemoRollsBackOnMidSequenceFailure(t *testing.T) {
	resetOverrides(t)
	l := &callLog{failOn: "command network"}
	c := &Client{Serial: "R58", run: l.runner()}
	err := c.StatusBarDemo(context.Background(), true, StatusBarOptions{Clock: "1200"})
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("error should keep original and mention rollback: %v", err)
	}
	if !l.has("command exit") {
		t.Fatalf("no exit attempted: %v", l.calls)
	}
	if len(Overrides()) != 0 {
		t.Fatalf("rolled-back demo still recorded: %v", Overrides())
	}
}

func TestStatusBarDemoRollbackFailureStaysRecorded(t *testing.T) {
	resetOverrides(t)
	// enter succeeds; every later demo command, including exit, fails.
	c := &Client{Serial: "R58", run: func(ctx context.Context, args ...string) ([]byte, error) {
		j := strings.Join(args, " ")
		if strings.Contains(j, "command enter") || !strings.Contains(j, "command") {
			return nil, nil
		}
		return nil, errors.New("boom")
	}}
	err := c.StatusBarDemo(context.Background(), true, StatusBarOptions{Clock: "1200"})
	if err == nil || !strings.Contains(err.Error(), "rollback failed") {
		t.Fatalf("want rollback failed in error: %v", err)
	}
	if len(Overrides()) != 1 {
		t.Fatalf("failed rollback must stay recorded: %v", Overrides())
	}
}

func TestStatusBarDemoEnterFailureNotRecorded(t *testing.T) {
	resetOverrides(t)
	l := &callLog{failOn: "command enter"}
	c := &Client{Serial: "R58", run: l.runner()}
	if err := c.StatusBarDemo(context.Background(), true, StatusBarOptions{}); err == nil {
		t.Fatal("want error")
	}
	if len(Overrides()) != 0 || l.has("command exit") {
		t.Fatalf("enter failed: nothing to roll back; got %v %v", Overrides(), l.calls)
	}
}

func TestStatusBarDemoSuccessRecordsAndExitClears(t *testing.T) {
	resetOverrides(t)
	l := &callLog{}
	c := &Client{Serial: "R58", run: l.runner()}
	if err := c.StatusBarDemo(context.Background(), true, StatusBarOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(Overrides()) != 1 {
		t.Fatalf("not recorded: %v", Overrides())
	}
	if err := c.StatusBarDemo(context.Background(), false, StatusBarOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(Overrides()) != 0 {
		t.Fatalf("exit did not clear: %v", Overrides())
	}
}

func TestRestoreOverridesUndoesOnlyBatteryAndDemo(t *testing.T) {
	resetOverrides(t)
	l := &callLog{}
	newClient = func(serial string) *Client { return &Client{Serial: serial, run: l.runner()} }
	NoteOverride("R58", OverrideBattery)
	NoteOverride("R58", OverrideStatusBarDemo)
	NoteOverride("R58", OverrideStayAwake)
	NoteOverride("R58", OverrideDeviceLock)

	errs := RestoreOverrides(context.Background())
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if !l.has("dumpsys battery reset") || !l.has("command exit") {
		t.Fatalf("battery/demo not undone: %v", l.calls)
	}
	if l.has("stayon") || l.has("locksettings") {
		t.Fatalf("must not touch stay_awake/lock: %v", l.calls)
	}
	left := Overrides()
	if len(left) != 2 {
		t.Fatalf("stay_awake and lock must stay recorded: %v", left)
	}
}

func TestRestoreOverridesIsBoundedAndCollectsErrors(t *testing.T) {
	resetOverrides(t)
	oldPer, oldTotal := restoreCallTimeout, restoreTotalTimeout
	restoreCallTimeout, restoreTotalTimeout = 50*time.Millisecond, 120*time.Millisecond
	t.Cleanup(func() { restoreCallTimeout, restoreTotalTimeout = oldPer, oldTotal })

	hang := func(ctx context.Context, args ...string) ([]byte, error) {
		<-ctx.Done() // a wedged adb: only the deadline frees it
		return nil, ctx.Err()
	}
	newClient = func(serial string) *Client { return &Client{Serial: serial, run: hang} }
	for _, s := range []string{"a", "b", "c", "d", "e"} {
		NoteOverride(s, OverrideBattery)
		NoteOverride(s, OverrideStatusBarDemo)
	}
	start := time.Now()
	errs := RestoreOverrides(context.Background())
	if d := time.Since(start); d > time.Second {
		t.Fatalf("not bounded: took %v", d)
	}
	if len(errs) == 0 {
		t.Fatal("want collected errors")
	}
	if len(Overrides()) != 10 {
		t.Fatalf("failed undo must stay recorded: %d", len(Overrides()))
	}
}

func TestDoctorOverridesSilentWhenEmptyAndListsLeftovers(t *testing.T) {
	resetOverrides(t)
	if s := doctorOverrides(); s != "" {
		t.Fatalf("want empty, got %q", s)
	}
	NoteOverride("R58", OverrideBattery)
	NoteOverride("R58", OverrideDeviceLock)
	s := doctorOverrides()
	for _, want := range []string{"R58", "battery", "reset=true", "device_lock", "clear_device_lock"} {
		if !strings.Contains(s, want) {
			t.Errorf("report missing %q:\n%s", want, s)
		}
	}
}
