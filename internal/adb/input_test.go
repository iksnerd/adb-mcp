package adb

import (
	"testing"

	"github.com/iksnerd/adb_mcp/internal/uiauto"
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
