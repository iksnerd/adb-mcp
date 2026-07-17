package android

import (
	"context"
	"fmt"
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
func CaptureScreen(ctx context.Context, serial string, maxDim int) (ScreenCapture, error) {
	var raw []byte
	var black bool
	attempts := 0
	for {
		attempts++
		var err error
		raw, err = runAdbBytes(ctx, serial, "exec-out", "screencap", "-p")
		if err != nil {
			return ScreenCapture{}, err
		}
		if len(raw) == 0 {
			return ScreenCapture{}, fmt.Errorf("empty screenshot from device %s", serial)
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
		res.SecureWindow = hasSecureWindow(ctx, serial)
		res.ScreenOff = !isScreenAwake(ctx, serial)
	}
	return res, nil
}

// Screenshot captures the screen with `exec-out screencap -p` (avoids the
// CRLF corruption of `shell screencap`) and downscales it so its largest
// dimension is at most maxDim. It returns the PNG bytes and their dimensions.
// Prefer CaptureScreen, which also detects and diagnoses black frames.
func Screenshot(ctx context.Context, serial string, maxDim int) (png []byte, w, h int, err error) {
	raw, err := runAdbBytes(ctx, serial, "exec-out", "screencap", "-p")
	if err != nil {
		return nil, 0, 0, err
	}
	if len(raw) == 0 {
		return nil, 0, 0, fmt.Errorf("empty screenshot from device %s", serial)
	}
	out, w, h := downscalePNG(raw, maxDim)
	return out, w, h, nil
}

// hasSecureWindow reports whether any window on screen carries FLAG_SECURE,
// which the OS renders as a black region in a screenshot. WindowManager prints
// each window's flags in human-readable form on an `fl=` line, where
// FLAG_SECURE appears as the standalone token "SECURE".
func hasSecureWindow(ctx context.Context, serial string) bool {
	out, err := runAdb(ctx, serial, "shell", "dumpsys", "window")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "fl=") {
			continue
		}
		for _, tok := range strings.Fields(strings.TrimPrefix(line, "fl=")) {
			if tok == "SECURE" {
				return true
			}
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
func TopWindow(ctx context.Context, serial string) (string, error) {
	out, err := runAdb(ctx, serial, "shell", "dumpsys", "window", "windows")
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
		for _, line := range strings.Split(dump, "\n") {
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
func isScreenAwake(ctx context.Context, serial string) bool {
	out, err := runAdb(ctx, serial, "shell", "dumpsys", "power")
	if err != nil {
		return true
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "mWakefulness="); ok {
			f := strings.Fields(v)
			return len(f) == 0 || strings.EqualFold(f[0], "Awake")
		}
	}
	return true
}
