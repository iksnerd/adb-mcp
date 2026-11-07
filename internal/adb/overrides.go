package adb

import (
	"context"
	"fmt"
	"regexp"
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
	// prior holds, per override, the value of the setting this process
	// overwrote, when it could be read ("null" = the key was unset). Absent
	// means unknown, and an override with an unknown prior is not restored.
	prior map[Override]string
}{m: map[Override]struct{}{}, prior: map[Override]string{}}

// untrackedTools lists the tools that leave device state behind but have no
// override kind, each with the reason. It makes the decision explicit: a tool
// that changes device state either has a kind above or a line here. Console
// state lives in the emulator process and goes when it does; the others are
// harmless or have no prior value we could restore.
var untrackedTools = map[string]string{
	"set_dark_mode": "persisted UI night mode; the previous mode is not read, so there is nothing to restore it to",
	"set_location":  "emulator console geo fix; dies with the emulator process",
	"cellular":      "emulator console radio and network profile; dies with the emulator process",
	"set_sensor":    "emulator console sensor values; dies with the emulator process",
	"rotate_screen": "emulator console rotation toggle; dies with the emulator process",
}

// UntrackedTools returns the documented tool -> reason table of state changes
// that are deliberately not tracked as overrides.
func UntrackedTools() map[string]string {
	out := make(map[string]string, len(untrackedTools))
	for k, v := range untrackedTools {
		out[k] = v
	}
	return out
}

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
	delete(overrides.prior, Override{serial, kind})
}

func clearAllOverrides() {
	overrides.mu.Lock()
	defer overrides.mu.Unlock()
	overrides.m = map[Override]struct{}{}
	overrides.prior = map[Override]string{}
}

// overrideState returns the stored prior value and whether kind is recorded.
func overrideState(serial string, kind OverrideKind) (prior string, recorded bool) {
	overrides.mu.Lock()
	defer overrides.mu.Unlock()
	o := Override{serial, kind}
	_, recorded = overrides.m[o]
	return overrides.prior[o], recorded
}

// noteOverridePrior records kind and, if it was not recorded yet, the setting
// value in force before this process changed it. A repeat call keeps the first
// prior: the value then on the device is our own. An empty prior means unknown.
func noteOverridePrior(serial string, kind OverrideKind, prior string) {
	overrides.mu.Lock()
	defer overrides.mu.Unlock()
	o := Override{serial, kind}
	if _, ok := overrides.m[o]; !ok && prior != "" {
		overrides.prior[o] = prior
	}
	overrides.m[o] = struct{}{}
}

// stayOnKey is the global setting `svc power stayon` writes.
const stayOnKey = "stay_on_while_plugged_in"

var settingValueRe = regexp.MustCompile(`^(\d+|null)$`)

// readSetting returns the current value of a global setting: digits, "null"
// when unset, or "" when it cannot be read or is not a plain integer. Device
// output only ever reaches a later command through this filter.
func (c *Client) readSetting(ctx context.Context, key string) string {
	out, err := c.adb(ctx, "shell", "settings", "get", "global", key)
	if err != nil {
		return ""
	}
	v := strings.TrimSpace(out)
	if !settingValueRe.MatchString(v) {
		return ""
	}
	return v
}

// restoreSetting puts a global setting back to prior as read by readSetting.
// An empty prior (unknown) does nothing.
func (c *Client) restoreSetting(ctx context.Context, key, prior string) error {
	switch prior {
	case "":
		return nil
	case "null":
		_, err := c.adb(ctx, "shell", "settings", "delete", "global", key)
		return err
	default:
		_, err := c.adb(ctx, "shell", "settings", "put", "global", key, prior)
		return err
	}
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
// reverse: battery (reset), status bar demo (exit, then the prior
// sysui_demo_allowed) and stay_awake when its prior setting was captured.
// device_lock (needs the old credential) and a stay_awake with an unknown
// prior are left recorded and untouched.
// Each adb call has its own short timeout and the whole pass is bounded, so a
// wedged adb cannot hold up shutdown. Failures are collected, not fatal, and an
// override whose undo failed stays recorded.
func RestoreOverrides(ctx context.Context) []error {
	ctx, cancel := context.WithTimeout(ctx, restoreTotalTimeout)
	defer cancel()
	var errs []error
	for _, o := range Overrides() {
		prior, _ := overrideState(o.Serial, o.Kind)
		switch {
		case o.Kind == OverrideBattery, o.Kind == OverrideStatusBarDemo:
		case o.Kind == OverrideStayAwake && prior != "":
		default:
			continue
		}
		if err := ctx.Err(); err != nil {
			errs = append(errs, fmt.Errorf("%s %s: %w", o.Serial, o.Kind, err))
			continue
		}
		callCtx, callCancel := context.WithTimeout(ctx, restoreCallTimeout)
		c := newClient(o.Serial)
		var err error
		switch o.Kind {
		case OverrideBattery:
			err = c.SetBattery(callCtx, nil, nil, true)
		case OverrideStatusBarDemo:
			err = c.demoBroadcast(callCtx, "exit")
			if err == nil {
				err = c.restoreDemoAllowed(callCtx, prior)
			}
			if err == nil {
				ClearOverride(o.Serial, o.Kind)
			}
		case OverrideStayAwake:
			err = c.restoreSetting(callCtx, stayOnKey, prior)
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
