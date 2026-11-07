package adb

import (
	"context"
	"strings"
	"testing"

	"github.com/iksnerd/adb-mcp/internal/uiauto"
)

func TestEscapeInputText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "hello", `'hello'`},
		{"spaces", "hello world", `'hello world'`},
		{"shell metachars", "a&b|c;d>e<f?(g)", `'a&b|c;d>e<f?(g)'`},
		{"dollar and backtick", "$PATH `id`", "'$PATH `id`'"},
		{"double quote", `say "hi"`, `'say "hi"'`},
		{"single quote", "it's", `'it'\''s'`},
		{"empty", "", `''`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := escapeInputText(c.in); got != c.want {
				t.Errorf("escapeInputText(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestAllDigitsPresent covers the gate the PIN-pad read-retry loops on: it must
// require EVERY distinct digit to be findable, so a dump that captured the pad
// only partially (the keyguard bouncer flickers in uiautomator) triggers a retry
// instead of a mis-tap.
func TestAllDigitsPresent(t *testing.T) {
	pad := func(digits ...string) []uiauto.Element {
		var e []uiauto.Element
		for _, d := range digits {
			e = append(e, uiauto.Element{Text: d, Clickable: true})
		}
		return e
	}
	full := pad("1", "2", "3", "4", "5", "6", "7", "8", "9", "0")
	if !allDigitsPresent(full, "1234") {
		t.Error("expected all of 1234 present in a full pad")
	}
	if allDigitsPresent(pad("1", "3", "4"), "1234") {
		t.Error("expected a partial pad (missing 2) to fail the gate")
	}
	if allDigitsPresent(nil, "1") {
		t.Error("expected an empty dump to fail the gate")
	}
	// Repeated digits only need the one key present.
	if !allDigitsPresent(pad("0", "1"), "1111") {
		t.Error("expected repeated digit 1 to pass when key 1 is present")
	}
}

func TestDialpadPoint(t *testing.T) {
	// A 300x400 pad at origin: colW=100, rowH=100, so centers land at 50/150/250
	// horizontally and 50/150/250/350 vertically.
	b := uiauto.Bounds{X1: 0, Y1: 0, X2: 300, Y2: 400}
	cases := map[rune]uiauto.Point{
		'1': {X: 50, Y: 50}, '2': {X: 150, Y: 50}, '3': {X: 250, Y: 50},
		'4': {X: 50, Y: 150}, '5': {X: 150, Y: 150}, '6': {X: 250, Y: 150},
		'7': {X: 50, Y: 250}, '8': {X: 150, Y: 250}, '9': {X: 250, Y: 250},
		'0': {X: 150, Y: 350},
	}
	for d, want := range cases {
		got, ok := dialpadPoint(b, d)
		if !ok || got != want {
			t.Errorf("dialpadPoint(%q) = %+v ok=%v, want %+v", string(d), got, ok, want)
		}
	}
	if _, ok := dialpadPoint(b, 'x'); ok {
		t.Errorf("expected non-digit to fail")
	}
}

func TestDialpadPointOffset(t *testing.T) {
	// Pad not at origin: [100,200]-[400,600]. colW=100, rowH=100.
	b := uiauto.Bounds{X1: 100, Y1: 200, X2: 400, Y2: 600}
	got, _ := dialpadPoint(b, '5')
	if want := (uiauto.Point{X: 250, Y: 350}); got != want {
		t.Errorf("dialpadPoint('5') = %+v, want %+v", got, want)
	}
	got, _ = dialpadPoint(b, '0')
	if want := (uiauto.Point{X: 250, Y: 550}); got != want {
		t.Errorf("dialpadPoint('0') = %+v, want %+v", got, want)
	}
}

// `input text` turns a literal %s into a space and cannot type non-ASCII; both
// used to return success with the wrong text on the device.
func TestInputTextRejectsWhatInputTextCannotType(t *testing.T) {
	for _, in := range []string{"100%sure", "café", "hi \U0001F600"} {
		c, f := newFake("")
		err := c.InputText(context.Background(), in)
		if err == nil {
			t.Errorf("InputText(%q) should be rejected", in)
		}
		if f.last() != nil {
			t.Errorf("InputText(%q) must not reach the device, ran %q", in, f.last())
		}
		if err != nil && !strings.Contains(err.Error(), "input text") {
			t.Errorf("error should explain itself, got %v", err)
		}
	}
	c, f := newFake("")
	if err := c.InputText(context.Background(), "100% sure"); err != nil || f.last() == nil {
		t.Errorf("plain ASCII with a bare %% must still type: err=%v", err)
	}
}
