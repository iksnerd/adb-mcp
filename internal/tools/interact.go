package tools

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/iksnerd/adb_mcp/internal/android"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---- Arguments ----

type tapArgs struct {
	serialArg
	X            int   `json:"x" jsonschema:"X coordinate in true device pixels."`
	Y            int   `json:"y" jsonschema:"Y coordinate in true device pixels."`
	VerifyChange *bool `json:"verify_change,omitempty" jsonschema:"Also report whether the UI hierarchy changed after the tap (ui_changed: true/false). Costs two extra hierarchy reads (~2-3s); use when a tap silently doing nothing would send you down the wrong path."`
}

type tapTextArgs struct {
	serialArg
	Text    string `json:"text" jsonschema:"Text or content-description to find."`
	Partial *bool  `json:"partial,omitempty" jsonschema:"Substring match instead of exact. Default true."`
}

type swipeArgs struct {
	serialArg
	X1         *int `json:"x1,omitempty" jsonschema:"Start X in true device pixels (alias: x)."`
	Y1         *int `json:"y1,omitempty" jsonschema:"Start Y in true device pixels (alias: y)."`
	X2         int  `json:"x2" jsonschema:"End X in true device pixels."`
	Y2         int  `json:"y2" jsonschema:"End Y in true device pixels."`
	X          *int `json:"x,omitempty" jsonschema:"Alias for x1 (start X)."`
	Y          *int `json:"y,omitempty" jsonschema:"Alias for y1 (start Y)."`
	DurationMS int  `json:"duration_ms,omitempty" jsonschema:"Swipe duration in ms. Default 300."`
}

type dragArgs struct {
	serialArg
	X1         int `json:"x1" jsonschema:"Start X in true device pixels."`
	Y1         int `json:"y1" jsonschema:"Start Y in true device pixels."`
	X2         int `json:"x2" jsonschema:"End X in true device pixels."`
	Y2         int `json:"y2" jsonschema:"End Y in true device pixels."`
	DurationMS int `json:"duration_ms,omitempty" jsonschema:"Drag duration in ms. Default 400."`
}

type keyComboArgs struct {
	serialArg
	Keys   []string `json:"keys,omitempty" jsonschema:"Keys to press together, modifier(s) first, e.g. [\"ctrl\",\"a\"] or [\"alt\",\"tab\"]. Each is a key name (ctrl, alt, shift, meta, a-z, enter, tab, ...) or a raw keycode number. Needs at least 2. Omit if preset is given."`
	Preset string   `json:"preset,omitempty" jsonschema:"Named combo shortcut (select_all, copy, paste, cut, undo, redo, save, find) that expands to the right chord — use this instead of keys when a name will do."`
}

type inputTextArgs struct {
	serialArg
	Text string `json:"text" jsonschema:"Text to type into the focused field."`
}

type pressKeyArgs struct {
	serialArg
	Key          string `json:"key" jsonschema:"Key name (enter, back, home, menu, tab, del, escape, up, down, left, right, ...) or a raw keycode number."`
	VerifyChange *bool  `json:"verify_change,omitempty" jsonschema:"Also report whether the UI hierarchy changed after the key press (ui_changed: true/false). Costs two extra hierarchy reads (~2-3s); use when the key may be silently consumed (e.g. back while a biometric prompt is up)."`
}

type longPressArgs struct {
	serialArg
	X          int `json:"x" jsonschema:"X coordinate in true device pixels."`
	Y          int `json:"y" jsonschema:"Y coordinate in true device pixels."`
	DurationMS int `json:"duration_ms,omitempty" jsonschema:"Hold duration in ms. Default 600."`
}

type enterPINArgs struct {
	serialArg
	Digits string   `json:"digits" jsonschema:"The digits to enter, e.g. \"1234\"."`
	Grid   *gridArg `json:"grid,omitempty" jsonschema:"Optional bounding box {x1,y1,x2,y2} of the PIN pad. Provide this for custom-drawn (React Native / Skia) pads whose keys are invisible to describe_ui: digits are placed on a standard 3x4 dialpad grid (1-2-3 / 4-5-6 / 7-8-9 / _-0-_) inside the box."`
	Coords string   `json:"coords,omitempty" jsonschema:"Optional explicit per-digit tap points as 'digit:x,y' pairs separated by ';', e.g. '1:540,1600;2:640,1600'. Overrides grid and hierarchy for the digits given. Use when the pad is not a regular grid."`
}

type gridArg struct {
	X1 int `json:"x1" jsonschema:"Left edge of the pad in true device pixels."`
	Y1 int `json:"y1" jsonschema:"Top edge of the pad in true device pixels."`
	X2 int `json:"x2" jsonschema:"Right edge of the pad in true device pixels."`
	Y2 int `json:"y2" jsonschema:"Bottom edge of the pad in true device pixels."`
}

// ---- Handlers ----

func tap(ctx context.Context, in tapArgs) (*mcp.CallToolResult, error) {
	serial, err := resolve(ctx, in.Serial)
	if err != nil {
		return nil, err
	}
	verdict, err := withChangeCheck(ctx, serial, boolOr(in.VerifyChange, false), func() error {
		return android.Tap(ctx, serial, in.X, in.Y)
	})
	if err != nil {
		return nil, err
	}
	return text("Tapped (%d,%d).%s", in.X, in.Y, verdict), nil
}

func tapOnText(ctx context.Context, in tapTextArgs) (*mcp.CallToolResult, error) {
	serial, err := resolve(ctx, in.Serial)
	if err != nil {
		return nil, err
	}
	snap, err := android.DescribeUI(ctx, serial, android.FilterAuto)
	if err != nil {
		return nil, err
	}
	e, ok := android.FindByText(snap.Elements, in.Text, boolOr(in.Partial, true))
	if !ok {
		if snap.TopWindow != "" {
			return nil, fmt.Errorf("no element matching %q found on screen (focused window: %s — if that is a system overlay, the app's UI is occluded)", in.Text, snap.TopWindow)
		}
		return nil, fmt.Errorf("no element matching %q found on screen", in.Text)
	}
	if err := android.Tap(ctx, serial, e.Center.X, e.Center.Y); err != nil {
		return nil, err
	}
	label := e.Text
	if label == "" {
		label = e.Desc
	}
	return text("Tapped %q at (%d,%d).", label, e.Center.X, e.Center.Y), nil
}

func swipe(ctx context.Context, in swipeArgs) (*mcp.CallToolResult, error) {
	serial, err := resolve(ctx, in.Serial)
	if err != nil {
		return nil, err
	}
	x1 := firstInt(in.X1, in.X) // x is an accepted alias for x1
	y1 := firstInt(in.Y1, in.Y)
	if x1 == nil || y1 == nil {
		return nil, fmt.Errorf("swipe needs a start point — provide x1, y1 (x and y are accepted aliases for x1 and y1)")
	}
	if err := android.Swipe(ctx, serial, *x1, *y1, in.X2, in.Y2, in.DurationMS); err != nil {
		return nil, err
	}
	return text("Swiped (%d,%d)->(%d,%d).", *x1, *y1, in.X2, in.Y2), nil
}

func drag(ctx context.Context, in dragArgs) (*mcp.CallToolResult, error) {
	serial, err := resolve(ctx, in.Serial)
	if err != nil {
		return nil, err
	}
	if err := android.Drag(ctx, serial, in.X1, in.Y1, in.X2, in.Y2, in.DurationMS); err != nil {
		return nil, err
	}
	return text("Dragged (%d,%d)->(%d,%d).", in.X1, in.Y1, in.X2, in.Y2), nil
}

func inputKeyCombo(ctx context.Context, in keyComboArgs) (*mcp.CallToolResult, error) {
	serial, err := resolve(ctx, in.Serial)
	if err != nil {
		return nil, err
	}
	var codes []int
	var label string
	if in.Preset != "" {
		codes, err = android.ResolveCombo(in.Preset)
		if err != nil {
			return nil, err
		}
		label = in.Preset
	} else {
		if len(in.Keys) < 2 {
			return nil, fmt.Errorf("a key combination needs at least 2 keys, e.g. [\"ctrl\",\"a\"], or pass preset instead")
		}
		codes = make([]int, 0, len(in.Keys))
		for _, k := range in.Keys {
			code, err := android.ResolveKey(k)
			if err != nil {
				return nil, err
			}
			codes = append(codes, code)
		}
		label = strings.Join(in.Keys, "+")
	}
	if err := android.KeyCombo(ctx, serial, codes); err != nil {
		return nil, err
	}
	return text("Pressed %s together.", label), nil
}

func inputText(ctx context.Context, in inputTextArgs) (*mcp.CallToolResult, error) {
	serial, err := resolve(ctx, in.Serial)
	if err != nil {
		return nil, err
	}
	if err := android.InputText(ctx, serial, in.Text); err != nil {
		return nil, err
	}
	return text("Typed %q. (If a button below is now hidden by the keyboard, press_key escape or back first.)", in.Text), nil
}

func pressKey(ctx context.Context, in pressKeyArgs) (*mcp.CallToolResult, error) {
	serial, err := resolve(ctx, in.Serial)
	if err != nil {
		return nil, err
	}
	code, err := android.ResolveKey(in.Key)
	if err != nil {
		return nil, err
	}
	verdict, err := withChangeCheck(ctx, serial, boolOr(in.VerifyChange, false), func() error {
		return android.PressKey(ctx, serial, code)
	})
	if err != nil {
		return nil, err
	}
	return text("Pressed %s (keycode %d).%s", in.Key, code, verdict), nil
}

type waitArgs struct {
	Seconds float64 `json:"seconds" jsonschema:"How long to wait, in seconds. Fractions allowed; capped at 300."`
}

func wait(ctx context.Context, in waitArgs) (*mcp.CallToolResult, error) {
	secs := in.Seconds
	if secs <= 0 {
		return nil, fmt.Errorf("seconds must be positive")
	}
	if secs > 300 {
		secs = 300
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(time.Duration(secs * float64(time.Second))):
	}
	return text("Waited %gs.", secs), nil
}

// withChangeCheck runs act and, when verify is set, compares a UI-hierarchy
// fingerprint from before and after so the caller learns whether the action
// had any visible effect — a success-shaped reply for a swallowed event (e.g.
// back while a BiometricPrompt is up) is indistinguishable from a real one
// otherwise. The verdict string is empty when verify is off.
func withChangeCheck(ctx context.Context, serial string, verify bool, act func() error) (string, error) {
	if !verify {
		return "", act()
	}
	before, beforeErr := android.UISignature(ctx, serial)
	if err := act(); err != nil {
		return "", err
	}
	time.Sleep(600 * time.Millisecond)
	after, afterErr := android.UISignature(ctx, serial)
	switch {
	case beforeErr != nil || afterErr != nil:
		return " ui_changed: unknown (hierarchy read failed).", nil
	case before == after:
		return " ui_changed: false — the UI looks identical, so the event likely had no effect; check describe_ui's top window for a system overlay consuming input.", nil
	default:
		return " ui_changed: true.", nil
	}
}

func longPress(ctx context.Context, in longPressArgs) (*mcp.CallToolResult, error) {
	serial, err := resolve(ctx, in.Serial)
	if err != nil {
		return nil, err
	}
	if err := android.LongPress(ctx, serial, in.X, in.Y, in.DurationMS); err != nil {
		return nil, err
	}
	return text("Long-pressed (%d,%d).", in.X, in.Y), nil
}

func enterPIN(ctx context.Context, in enterPINArgs) (*mcp.CallToolResult, error) {
	serial, err := resolve(ctx, in.Serial)
	if err != nil {
		return nil, err
	}
	var grid *android.Bounds
	if in.Grid != nil {
		grid = &android.Bounds{X1: in.Grid.X1, Y1: in.Grid.Y1, X2: in.Grid.X2, Y2: in.Grid.Y2}
	}
	coords, err := parseCoords(in.Coords)
	if err != nil {
		return nil, err
	}
	if err := android.EnterPIN(ctx, serial, in.Digits, grid, coords); err != nil {
		return nil, err
	}
	return text("Entered %d digit(s).", len(in.Digits)), nil
}

// parseCoords turns "1:540,1600;2:640,1600" into a digit→point map.
func parseCoords(s string) (map[rune]android.Point, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	m := make(map[rune]android.Point)
	for _, pair := range strings.Split(s, ";") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		sep := strings.IndexAny(pair, ":=")
		if sep < 1 {
			return nil, fmt.Errorf("bad coords entry %q; expected 'digit:x,y'", pair)
		}
		digit := strings.TrimSpace(pair[:sep])
		if len(digit) != 1 || digit[0] < '0' || digit[0] > '9' {
			return nil, fmt.Errorf("bad digit in coords entry %q; expected a single 0-9", pair)
		}
		xy := strings.SplitN(pair[sep+1:], ",", 2)
		if len(xy) != 2 {
			return nil, fmt.Errorf("bad point in coords entry %q; expected 'x,y'", pair)
		}
		x, err1 := strconv.Atoi(strings.TrimSpace(xy[0]))
		y, err2 := strconv.Atoi(strings.TrimSpace(xy[1]))
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("non-integer coordinate in %q", pair)
		}
		m[rune(digit[0])] = android.Point{X: x, Y: y}
	}
	return m, nil
}

// firstInt returns the first non-nil pointer (used to resolve arg aliases).
func firstInt(vals ...*int) *int {
	for _, v := range vals {
		if v != nil {
			return v
		}
	}
	return nil
}
