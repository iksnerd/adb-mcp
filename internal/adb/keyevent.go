package adb

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// keyCodes maps human-friendly key names to Android keyevent codes. See
// https://developer.android.com/reference/android/view/KeyEvent for the full
// list; these are the ones useful when driving a UI by hand.
var keyCodes = map[string]int{
	"back":         4,
	"home":         3,
	"menu":         82,
	"enter":        66,
	"tab":          61,
	"space":        62,
	"del":          67, // backspace
	"backspace":    67,
	"forward_del":  112,
	"escape":       111,
	"esc":          111,
	"up":           19,
	"down":         20,
	"left":         21,
	"right":        22,
	"dpad_center":  23,
	"power":        26,
	"volume_up":    24,
	"volume_down":  25,
	"camera":       27,
	"search":       84,
	"app_switch":   187, // recent apps
	"notification": 83,

	// Modifiers — useful as the first key(s) of an input_key_combo chord.
	"ctrl":        113, // ctrl_left
	"ctrl_left":   113,
	"ctrl_right":  114,
	"alt":         57, // alt_left
	"alt_left":    57,
	"alt_right":   58,
	"shift":       59, // shift_left
	"shift_left":  59,
	"shift_right": 60,
	"meta":        117, // meta_left (Windows/Command)
	"meta_left":   117,
	"meta_right":  118,
	"caps_lock":   115,

	// Letters — handy as the action key in a chord (e.g. ctrl+a).
	"a": 29, "b": 30, "c": 31, "d": 32, "e": 33, "f": 34,
	"g": 35, "h": 36, "i": 37, "j": 38, "k": 39, "l": 40,
	"m": 41, "n": 42, "o": 43, "p": 44, "q": 45, "r": 46,
	"s": 47, "t": 48, "u": 49, "v": 50, "w": 51, "x": 52,
	"y": 53, "z": 54,
}

// presetCombos maps named shortcuts to the chord of key names that expands to
// them, so callers of input_key_combo don't need to know the individual keys.
var presetCombos = map[string][]string{
	"select_all": {"ctrl", "a"},
	"copy":       {"ctrl", "c"},
	"paste":      {"ctrl", "v"},
	"cut":        {"ctrl", "x"},
	"undo":       {"ctrl", "z"},
	"redo":       {"ctrl", "shift", "z"},
	"save":       {"ctrl", "s"},
	"find":       {"ctrl", "f"},
}

// ResolveCombo expands a named preset (case-insensitive) into resolved
// keyevent codes, one per key, via ResolveKey.
func ResolveCombo(name string) ([]int, error) {
	keys, ok := presetCombos[strings.TrimSpace(strings.ToLower(name))]
	if !ok {
		return nil, fmt.Errorf("unknown combo preset %q; use one of: %s (or pass keys explicitly)", name, strings.Join(PresetNames(), ", "))
	}
	codes := make([]int, 0, len(keys))
	for _, k := range keys {
		code, err := ResolveKey(k)
		if err != nil {
			return nil, err
		}
		codes = append(codes, code)
	}
	return codes, nil
}

// PresetNames returns the sorted list of supported combo presets.
func PresetNames() []string {
	names := make([]string, 0, len(presetCombos))
	for name := range presetCombos {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ResolveKey converts a named key (case-insensitive) or a raw integer keycode
// string into an Android keyevent code.
func ResolveKey(key string) (int, error) {
	k := strings.TrimSpace(strings.ToLower(key))
	if k == "" {
		return 0, fmt.Errorf("empty key")
	}
	if code, ok := keyCodes[k]; ok {
		return code, nil
	}
	if n, err := strconv.Atoi(k); err == nil {
		return n, nil
	}
	return 0, fmt.Errorf("unknown key %q; use a keycode number or one of: %s", key, strings.Join(KeyNames(), ", "))
}

// KeyNames returns the sorted list of supported named keys.
func KeyNames() []string {
	names := make([]string, 0, len(keyCodes))
	for name := range keyCodes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
