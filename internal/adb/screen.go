package adb

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// ScreenCapture is a screenshot plus a diagnosis of why it might be unusable.
// AllBlack is set when the captured image is (near-)entirely black; in that
// case SecureWindow/ScreenOff are best-effort probes of the likely cause so the
// caller can react (e.g. fall back to describe_ui) instead of guessing.
type ScreenCapture struct {
	PNG          []byte
	Width        int
	Height       int
	AllBlack     bool
	SecureWindow bool // a FLAG_SECURE window is on screen (OS blanks it black)
	ScreenOff    bool // the display is asleep/dozing
	Attempts     int  // how many grabs it took (>1 means black retries happened)
}

// blackRetries is how many extra times CaptureScreen re-grabs after an
// all-black frame — screencap intermittently returns a black image for a
// perfectly normal screen, and a moment later succeeds.
const blackRetries = 2

// CaptureScreen grabs the screen and, if the frame comes back all black,
// retries a couple of times (the intermittent-black case) before giving up and
// diagnosing the likely cause (FLAG_SECURE content or a sleeping display).
//
// displayID, when non-empty, is a PHYSICAL display id (see ResolveDisplay) that
// picks a specific screen on a multi-display device — e.g. a foldable's cover
// panel; empty captures the default (built-in) display.
func (c *Client) CaptureScreen(ctx context.Context, maxDim int, displayID string) (ScreenCapture, error) {
	var raw []byte
	var black bool
	attempts := 0
	for {
		attempts++
		var err error
		raw, err = c.screencap(ctx, displayID)
		if err != nil {
			return ScreenCapture{}, err
		}
		if len(raw) == 0 {
			return ScreenCapture{}, fmt.Errorf("empty screenshot from device %s", c.Serial)
		}
		black = isMostlyBlack(raw)
		if !black || attempts > blackRetries {
			break
		}
		time.Sleep(400 * time.Millisecond)
	}

	out, w, h := downscalePNG(raw, maxDim)
	res := ScreenCapture{PNG: out, Width: w, Height: h, Attempts: attempts, AllBlack: black}
	if black {
		// Best-effort — ignore probe errors, they only enrich the diagnosis.
		res.SecureWindow = c.hasSecureWindow(ctx)
		res.ScreenOff = !c.isScreenAwake(ctx)
	}
	return res, nil
}

// screencap runs `exec-out screencap -p` (avoids the CRLF corruption of
// `shell screencap`) and returns the PNG bytes with any leading non-PNG prefix
// stripped — a multi-display device prepends a warning line to stdout that would
// otherwise corrupt the header (see pngFromScreencap). displayID, when set,
// targets one physical display via `-d`.
func (c *Client) screencap(ctx context.Context, displayID string) ([]byte, error) {
	args := []string{"exec-out", "screencap", "-p"}
	if displayID != "" {
		args = append(args, "-d", displayID)
	}
	raw, err := c.adbBytes(ctx, args...)
	if err != nil {
		return nil, err
	}
	return pngFromScreencap(raw), nil
}

// Screenshot captures the screen and downscales it so its largest dimension is
// at most maxDim. It returns the PNG bytes and their dimensions. Prefer
// CaptureScreen, which also detects and diagnoses black frames.
func (c *Client) Screenshot(ctx context.Context, maxDim int) (png []byte, w, h int, err error) {
	raw, err := c.screencap(ctx, "")
	if err != nil {
		return nil, 0, 0, err
	}
	if len(raw) == 0 {
		return nil, 0, 0, fmt.Errorf("empty screenshot from device %s", c.Serial)
	}
	out, w, h := downscalePNG(raw, maxDim)
	return out, w, h, nil
}

// Display is one physical display on the device, as reported by SurfaceFlinger.
type Display struct {
	PhysicalID string // what `screencap -d` wants — NOT the logical display id
	Index      int    // HWC display index; 0 is the built-in/primary panel
	Name       string // e.g. "EMU_display_0"
}

// ListDisplays enumerates the device's physical displays. `screencap -d` keys
// off the physical id (a large opaque number), not the logical display id 0/1
// that `cmd display get-displays` shows — passing the logical id makes screencap
// fail outright — so this is the authoritative source for a per-display capture.
func (c *Client) ListDisplays(ctx context.Context) ([]Display, error) {
	out, err := c.adb(ctx, "shell", "dumpsys", "SurfaceFlinger", "--display-id")
	if err != nil {
		return nil, err
	}
	return parseDisplays(out), nil
}

// displayLineRe matches a SurfaceFlinger --display-id line:
//
//	Display 4619827259835644672 (HWC display 0): port=0 ... displayName="EMU_display_0"
var displayLineRe = regexp.MustCompile(`Display\s+(\d+)\s+\(HWC display\s+(\d+)\)`)

// parseDisplays turns `dumpsys SurfaceFlinger --display-id` output into the
// physical displays it lists, in report order. Pure — unit-tested directly.
func parseDisplays(out string) []Display {
	var ds []Display
	for line := range strings.SplitSeq(out, "\n") {
		m := displayLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		idx, _ := strconv.Atoi(m[2])
		d := Display{PhysicalID: m[1], Index: idx}
		if n := displayNameRe.FindStringSubmatch(line); n != nil {
			d.Name = n[1]
		}
		ds = append(ds, d)
	}
	return ds
}

var displayNameRe = regexp.MustCompile(`displayName="([^"]*)"`)

// ResolveDisplay turns a caller's display selector into a physical display id
// for `screencap -d`. It accepts:
//   - "" → "" (the default display, no -d)
//   - an alias: "primary"/"inner"/"main" (HWC index 0), "cover"/"outer"/
//     "secondary" (HWC index 1)
//   - a bare HWC index ("0", "1")
//   - a raw physical id (matched against the enumerated displays)
//
// Unknown selectors return an error listing the available displays rather than
// silently capturing the wrong screen.
func (c *Client) ResolveDisplay(ctx context.Context, sel string) (string, error) {
	sel = strings.TrimSpace(sel)
	if sel == "" {
		return "", nil
	}
	ds, err := c.ListDisplays(ctx)
	if err != nil {
		return "", err
	}
	return resolveDisplay(sel, ds)
}

// resolveDisplay is the pure core of ResolveDisplay (selector + display list →
// physical id), split out so it is unit-tested without a device.
func resolveDisplay(sel string, ds []Display) (string, error) {
	if len(ds) == 0 {
		return "", fmt.Errorf("no physical displays reported by SurfaceFlinger")
	}
	wantIdx := -1
	switch strings.ToLower(sel) {
	case "primary", "inner", "main", "default":
		wantIdx = 0
	case "cover", "outer", "secondary", "external":
		wantIdx = 1
	}
	for _, d := range ds {
		if d.PhysicalID == sel { // already a physical id
			return d.PhysicalID, nil
		}
		if wantIdx >= 0 && d.Index == wantIdx {
			return d.PhysicalID, nil
		}
	}
	// A bare HWC index that wasn't an alias.
	if n, err := strconv.Atoi(sel); err == nil {
		for _, d := range ds {
			if d.Index == n {
				return d.PhysicalID, nil
			}
		}
	}
	var avail []string
	for _, d := range ds {
		avail = append(avail, fmt.Sprintf("%d(%s,%s)", d.Index, d.Name, d.PhysicalID))
	}
	return "", fmt.Errorf("no display matching %q; available: %s (use an index, a name alias like inner/cover, or a physical id)", sel, strings.Join(avail, ", "))
}

// StayAwake keeps the display from dozing while the device is plugged in
// (svc power stayon true), or restores the normal timeout (false). Emulators
// always report as charging, so `true` holds the screen on for a whole driving
// session — the fix for an AVD whose framebuffer sleeps mid-flow and blanks
// screenshots to black. On a real unplugged device it has no effect until the
// device is charging.
func (c *Client) StayAwake(ctx context.Context, on bool) error {
	val := "false"
	if on {
		val = "true"
	}
	_, err := c.adb(ctx, "shell", "svc", "power", "stayon", val)
	return err
}

// hasSecureWindow reports whether any window on screen carries FLAG_SECURE,
// which the OS renders as a black region in a screenshot. WindowManager prints
// each window's flags in human-readable form on an `fl=` line, where
// FLAG_SECURE appears as the standalone token "SECURE".
func (c *Client) hasSecureWindow(ctx context.Context) bool {
	out, err := c.adb(ctx, "shell", "dumpsys", "window")
	if err != nil {
		return false
	}
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "fl=") {
			continue
		}
		if slices.Contains(strings.Fields(strings.TrimPrefix(line, "fl=")), "SECURE") {
			return true
		}
	}
	return false
}

// TopWindow returns the name of the currently focused window, e.g.
// "com.example.app/com.example.app.MainActivity" or
// "com.android.systemui.biometrics.ui.BiometricPromptDialog". This is how a
// caller tells that a SYSTEM overlay (biometric prompt, permission dialog,
// notification shade) sits on top of the app it thinks it is driving — the
// UI hierarchy then belongs to that overlay, not the app.
func (c *Client) TopWindow(ctx context.Context) (string, error) {
	out, err := c.adb(ctx, "shell", "dumpsys", "window", "windows")
	if err != nil {
		return "", err
	}
	if w := parseCurrentFocus(out); w != "" {
		return w, nil
	}
	return "", fmt.Errorf("no focused window reported by WindowManager")
}

// parseCurrentFocus extracts the window name from a WindowManager dump line of
// the form "mCurrentFocus=Window{f8ec3b7 u0 com.pkg/com.pkg.Activity}". Falls
// back to mFocusedWindow (some Android versions) with the same shape.
func parseCurrentFocus(dump string) string {
	for _, key := range []string{"mCurrentFocus=", "mFocusedWindow="} {
		for line := range strings.SplitSeq(dump, "\n") {
			line = strings.TrimSpace(line)
			v, ok := strings.CutPrefix(line, key)
			if !ok || !strings.HasPrefix(v, "Window{") {
				continue
			}
			v = strings.TrimPrefix(v, "Window{")
			v = strings.TrimSuffix(strings.TrimSpace(v), "}")
			// "f8ec3b7 u0 com.pkg/com.pkg.Activity" — the name is everything
			// after the identity hash and user id.
			if fields := strings.Fields(v); len(fields) >= 3 {
				return strings.Join(fields[2:], " ")
			}
		}
	}
	return ""
}

// isScreenAwake reports whether the display is on (mWakefulness=Awake). If the
// state can't be read it assumes awake, so a probe failure never mislabels a
// live screen as off.
func (c *Client) isScreenAwake(ctx context.Context) bool {
	out, err := c.adb(ctx, "shell", "dumpsys", "power")
	if err != nil {
		return true
	}
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "mWakefulness="); ok {
			f := strings.Fields(v)
			return len(f) == 0 || strings.EqualFold(f[0], "Awake")
		}
	}
	return true
}
