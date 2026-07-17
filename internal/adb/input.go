package adb

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/iksnerd/adb_mcp/internal/uiauto"
)

// Tap taps a coordinate in true device pixels.
func (c *Client) Tap(ctx context.Context, x, y int) error {
	_, err := c.adb(ctx, "shell", "input", "tap", strconv.Itoa(x), strconv.Itoa(y))
	return err
}

// Swipe drags from (x1,y1) to (x2,y2) over durationMS milliseconds.
func (c *Client) Swipe(ctx context.Context, x1, y1, x2, y2, durationMS int) error {
	if durationMS <= 0 {
		durationMS = 300
	}
	_, err := c.adb(ctx, "shell", "input", "swipe",
		strconv.Itoa(x1), strconv.Itoa(y1), strconv.Itoa(x2), strconv.Itoa(y2), strconv.Itoa(durationMS))
	return err
}

// Drag performs a press-hold-move-release drag from (x1,y1) to (x2,y2) using
// `input draganddrop` (Android 11+). Unlike Swipe — which flings — this holds at
// the start point first, so it triggers drag handles, reorder-on-long-press
// lists, and drag-and-drop targets that a quick swipe skips over.
func (c *Client) Drag(ctx context.Context, x1, y1, x2, y2, durationMS int) error {
	if durationMS <= 0 {
		durationMS = 400
	}
	_, err := c.adb(ctx, "shell", "input", "draganddrop",
		strconv.Itoa(x1), strconv.Itoa(y1), strconv.Itoa(x2), strconv.Itoa(y2), strconv.Itoa(durationMS))
	return err
}

// KeyCombo presses several keycodes together as a chord via
// `input keycombination` (Android 11+), e.g. ctrl+a or alt+tab. Order matters:
// list the modifier(s) first, then the action key.
func (c *Client) KeyCombo(ctx context.Context, codes []int) error {
	if len(codes) < 2 {
		return fmt.Errorf("a key combination needs at least 2 keys")
	}
	args := []string{"shell", "input", "keycombination"}
	for _, code := range codes {
		args = append(args, strconv.Itoa(code))
	}
	_, err := c.adb(ctx, args...)
	return err
}

// LongPress presses and holds a point by issuing a same-point swipe with a long
// duration (Android has no dedicated long-press input verb).
func (c *Client) LongPress(ctx context.Context, x, y, durationMS int) error {
	if durationMS <= 0 {
		durationMS = 600
	}
	_, err := c.adb(ctx, "shell", "input", "swipe",
		strconv.Itoa(x), strconv.Itoa(y), strconv.Itoa(x), strconv.Itoa(y), strconv.Itoa(durationMS))
	return err
}

// InputText types text via the IME. The text is quoted for the device shell so
// spaces and metacharacters survive intact (see escapeInputText).
func (c *Client) InputText(ctx context.Context, text string) error {
	_, err := c.adb(ctx, "shell", "input", "text", escapeInputText(text))
	return err
}

// escapeInputText prepares text for `adb shell input text`. The argument is
// interpreted by the device shell before `input` sees it, so a bare string with
// spaces or metacharacters ($, backtick, quotes, &, ;, |, <, >, parens, ?, ...)
// would be split or mangled. Wrapping the whole string in single quotes
// neutralises all of them at once; an embedded single quote is emitted as the
// standard '\” sequence (close-quote, escaped quote, reopen-quote).
func escapeInputText(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// PressKey sends a keyevent by resolved code.
func (c *Client) PressKey(ctx context.Context, code int) error {
	_, err := c.adb(ctx, "shell", "input", "keyevent", strconv.Itoa(code))
	return err
}

// EnterPIN types digits on a PIN pad, tapping each key with a settle delay so it
// registers. It resolves each digit's tap point in priority order:
//
//  1. an explicit coords override for that digit, else
//  2. a standard 3x4 dialpad position computed from grid (the pad's bounding
//     box), else
//  3. the digit's element located in the UI hierarchy.
//
// The grid and coords paths exist because custom-drawn pads (React Native /
// Skia SDK pads) render their keys on a canvas that uiautomator cannot see, so
// the hierarchy lookup (3) finds nothing. For those, pass grid or coords.
//
// Blind-tap guard: grid/coords tap coordinates with no idea what is on screen.
// If the focused window is a system BiometricPrompt, those taps would land on
// the prompt — not a PIN pad — so EnterPIN refuses and says how to proceed.
func (c *Client) EnterPIN(ctx context.Context, digits string, grid *uiauto.Bounds, coords map[rune]uiauto.Point) error {
	if grid != nil || len(coords) > 0 {
		if top, err := c.TopWindow(ctx); err == nil && strings.Contains(strings.ToLower(top), "biometric") {
			return fmt.Errorf("a system biometric prompt has focus (%s) — there is no PIN pad to tap, and blind grid/coords taps would hit the prompt. Satisfy it with fingerprint_touch, or cancel it (tap its negative button via describe_ui/tap_on_text) to fall back to the PIN pad, then retry", top)
		}
	}
	var elems []uiauto.Element // lazily loaded only if we fall through to the hierarchy
	for _, d := range digits {
		if d < '0' || d > '9' {
			return fmt.Errorf("digits must be 0-9, got %q", string(d))
		}
		var pt uiauto.Point
		switch {
		case hasPoint(coords, d):
			pt = coords[d]
		case grid != nil:
			p, ok := dialpadPoint(*grid, d)
			if !ok {
				return fmt.Errorf("cannot place digit %q on a 3x4 dialpad grid", string(d))
			}
			pt = p
		default:
			if elems == nil {
				var err error
				elems, _, err = c.describeSettled(ctx, uiauto.FilterAuto)
				if err != nil {
					return err
				}
			}
			e, ok := uiauto.FindByText(elems, string(d), false)
			if !ok {
				return fmt.Errorf("digit %q not found in the UI hierarchy; the pad may be custom-drawn (RN/Skia) and invisible to describe_ui. Pass 'grid' (the pad's bounding box, for a standard 3x4 layout) or 'coords' (explicit per-digit x,y)", string(d))
			}
			pt = e.Center
		}
		if err := c.Tap(ctx, pt.X, pt.Y); err != nil {
			return err
		}
		time.Sleep(300 * time.Millisecond)
	}
	return nil
}

func hasPoint(m map[rune]uiauto.Point, d rune) bool {
	if m == nil {
		return false
	}
	_, ok := m[d]
	return ok
}

// dialpadPoint maps a digit to its center on a standard 3x4 phone dialpad laid
// out inside b: rows 1-2-3 / 4-5-6 / 7-8-9 / (blank)-0-(blank).
func dialpadPoint(b uiauto.Bounds, digit rune) (uiauto.Point, bool) {
	positions := map[rune][2]int{ // {col, row}
		'1': {0, 0}, '2': {1, 0}, '3': {2, 0},
		'4': {0, 1}, '5': {1, 1}, '6': {2, 1},
		'7': {0, 2}, '8': {1, 2}, '9': {2, 2},
		'0': {1, 3},
	}
	pos, ok := positions[digit]
	if !ok {
		return uiauto.Point{}, false
	}
	colW := (b.X2 - b.X1) / 3
	rowH := (b.Y2 - b.Y1) / 4
	return uiauto.Point{
		X: b.X1 + pos[0]*colW + colW/2,
		Y: b.Y1 + pos[1]*rowH + rowH/2,
	}, true
}
