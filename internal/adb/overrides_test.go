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

func TestSetBatteryValidationFailureRecordsNothing(t *testing.T) {
	resetOverrides(t)
	l := &callLog{}
	c := &Client{Serial: "R58", run: l.runner()}
	bad := 101
	if err := c.SetBattery(context.Background(), &bad, nil, false); err == nil {
		t.Fatal("want validation error")
	}
	if err := c.SetBattery(context.Background(), nil, nil, false); err == nil {
		t.Fatal("want validation error")
	}
	if len(Overrides()) != 0 || len(l.calls) != 0 {
		t.Fatalf("validation failure touched state: %v %v", Overrides(), l.calls)
	}
}

// A set that fails after a mutation was attempted may have changed the device
// (the level applied, the charging state did not), so it stays recorded.
func TestSetBatteryFailureAfterMutationStaysRecorded(t *testing.T) {
	resetOverrides(t)
	l := &callLog{failOn: "battery set ac"}
	c := &Client{Serial: "R58", run: l.runner()}
	lvl, charging := 5, true
	if err := c.SetBattery(context.Background(), &lvl, &charging, false); err == nil {
		t.Fatal("want error")
	}
	if !l.has("battery set level 5") {
		t.Fatalf("level never applied: %v", l.calls)
	}
	if len(Overrides()) != 1 {
		t.Fatalf("half-applied set must stay recorded: %v", Overrides())
	}
}

func TestSetBatteryRecordedBeforeFirstMutation(t *testing.T) {
	resetOverrides(t)
	var seen int
	c := &Client{Serial: "R58", run: func(context.Context, ...string) ([]byte, error) {
		seen = len(Overrides())
		return nil, errors.New("boom")
	}}
	lvl := 5
	_ = c.SetBattery(context.Background(), &lvl, nil, false)
	if seen != 1 {
		t.Fatalf("override not recorded before the mutating call (saw %d)", seen)
	}
}

// On an emulator the values were set through the console, so a reset also sends
// the console back to its automatic state.
func TestSetBatteryResetOnEmulatorRestoresConsoleState(t *testing.T) {
	resetOverrides(t)
	l := &callLog{}
	c := &Client{Serial: "emulator-5554", run: l.runner()}
	lvl := 5
	if err := c.SetBattery(context.Background(), &lvl, nil, false); err != nil {
		t.Fatal(err)
	}
	if err := c.SetBattery(context.Background(), nil, nil, true); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"dumpsys battery reset", "emu power capacity 100", "emu power ac on"} {
		if !l.has(want) {
			t.Errorf("reset missing %q: %v", want, l.calls)
		}
	}
	if len(Overrides()) != 0 {
		t.Fatalf("confirmed reset must clear: %v", Overrides())
	}
}

func TestSetBatteryResetFailureStaysRecorded(t *testing.T) {
	resetOverrides(t)
	l := &callLog{failOn: "power ac on"}
	c := &Client{Serial: "emulator-5554", run: l.runner()}
	NoteOverride(c.Serial, OverrideBattery)
	if err := c.SetBattery(context.Background(), nil, nil, true); err == nil {
		t.Fatal("want error")
	}
	if len(Overrides()) != 1 {
		t.Fatalf("unconfirmed reset must stay recorded: %v", Overrides())
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
