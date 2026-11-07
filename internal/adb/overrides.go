package adb

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// OverrideKind names a piece of device state this process changed that outlives
// the tool call that changed it.
type OverrideKind string

const (
	OverrideBattery       OverrideKind = "battery"
	OverrideStatusBarDemo OverrideKind = "status_bar_demo"
	OverrideStayAwake     OverrideKind = "stay_awake"
	OverrideDeviceLock    OverrideKind = "device_lock"
)

// Override is one recorded device-side change. It holds no credential: for
// device_lock the fact alone is stored, never the PIN/pattern/password.
type Override struct {
	Serial string
	Kind   OverrideKind
}

// undoHint says how the user reverses each kind with the tools.
var undoHint = map[OverrideKind]string{
	OverrideBattery:       "set_battery reset=true",
	OverrideStatusBarDemo: "set_status_bar enabled=false",
	OverrideStayAwake:     "stay_awake enabled=false",
	OverrideDeviceLock:    "clear_device_lock (needs the credential)",
}

var overrides = struct {
	mu sync.Mutex
	m  map[Override]struct{}
}{m: map[Override]struct{}{}}

// NoteOverride records that this process applied kind on serial. Idempotent.
func NoteOverride(serial string, kind OverrideKind) {
	overrides.mu.Lock()
	defer overrides.mu.Unlock()
	overrides.m[Override{serial, kind}] = struct{}{}
}

// ClearOverride removes a recorded override (the matching undo succeeded).
func ClearOverride(serial string, kind OverrideKind) {
	overrides.mu.Lock()
	defer overrides.mu.Unlock()
	delete(overrides.m, Override{serial, kind})
}

func clearAllOverrides() {
	overrides.mu.Lock()
	defer overrides.mu.Unlock()
	overrides.m = map[Override]struct{}{}
}

// Overrides returns the recorded overrides, sorted by serial then kind.
func Overrides() []Override {
	overrides.mu.Lock()
	out := make([]Override, 0, len(overrides.m))
	for o := range overrides.m {
		out = append(out, o)
	}
	overrides.mu.Unlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Serial != out[j].Serial {
			return out[i].Serial < out[j].Serial
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

// doctorOverrides renders the leftovers for the doctor report; empty when none.
func doctorOverrides() string {
	list := Overrides()
	if len(list) == 0 {
		return ""
	}
	var b strings.Builder
	for _, o := range list {
		fmt.Fprintf(&b, "⚠ device state left by this server (%s): %s — undo with %s\n", o.Serial, o.Kind, undoHint[o.Kind])
	}
	return strings.TrimRight(b.String(), "\n")
}

// Bounds for RestoreOverrides, vars so tests can shrink them.
var (
	restoreCallTimeout  = 3 * time.Second
	restoreTotalTimeout = 10 * time.Second
	// newClient builds the Client RestoreOverrides uses; tests swap it.
	newClient = New
)

// RestoreOverrides reverses, best effort, the overrides that are safe to
// reverse without knowing prior state: battery (dumpsys battery reset) and
// status bar demo (exit). stay_awake (the previous setting is unknown) and
// device_lock (needs the old credential) are left recorded and untouched.
// Each adb call has its own short timeout and the whole pass is bounded, so a
// wedged adb cannot hold up shutdown. Failures are collected, not fatal, and an
// override whose undo failed stays recorded.
func RestoreOverrides(ctx context.Context) []error {
	ctx, cancel := context.WithTimeout(ctx, restoreTotalTimeout)
	defer cancel()
	var errs []error
	for _, o := range Overrides() {
		if o.Kind != OverrideBattery && o.Kind != OverrideStatusBarDemo {
			continue
		}
		if err := ctx.Err(); err != nil {
			errs = append(errs, fmt.Errorf("%s %s: %w", o.Serial, o.Kind, err))
			continue
		}
		callCtx, callCancel := context.WithTimeout(ctx, restoreCallTimeout)
		c := newClient(o.Serial)
		var err error
		if o.Kind == OverrideBattery {
			err = c.SetBattery(callCtx, nil, nil, true)
		} else {
			err = c.demoBroadcast(callCtx, "exit")
			if err == nil {
				ClearOverride(o.Serial, o.Kind)
			}
		}
		callCancel()
		if err != nil {
			errs = append(errs, fmt.Errorf("%s %s: %w", o.Serial, o.Kind, err))
		}
	}
	return errs
}
