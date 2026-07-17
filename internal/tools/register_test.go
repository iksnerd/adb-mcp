package tools

import (
	"testing"

	"github.com/iksnerd/adb_mcp/internal/uiauto"
)

func TestParseCoords(t *testing.T) {
	m, err := parseCoords("1:540,1600; 2 : 640 , 1600 ;3=740,1600")
	if err != nil {
		t.Fatalf("parseCoords: %v", err)
	}
	want := map[rune]uiauto.Point{
		'1': {X: 540, Y: 1600},
		'2': {X: 640, Y: 1600},
		'3': {X: 740, Y: 1600},
	}
	if len(m) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(m), len(want), m)
	}
	for d, p := range want {
		if m[d] != p {
			t.Errorf("digit %q = %+v, want %+v", string(d), m[d], p)
		}
	}
}

func TestParseCoordsEmpty(t *testing.T) {
	m, err := parseCoords("   ")
	if err != nil || m != nil {
		t.Errorf("empty coords should be (nil,nil), got (%v,%v)", m, err)
	}
}

func TestParseCoordsErrors(t *testing.T) {
	for _, bad := range []string{"12:1,2", "x:1,2", "1:1", "1:a,b", "1600"} {
		if _, err := parseCoords(bad); err == nil {
			t.Errorf("parseCoords(%q) expected error, got nil", bad)
		}
	}
}

func TestTailLines(t *testing.T) {
	if got := tailLines("a\nb\nc", 5); got != "a\nb\nc" {
		t.Errorf("short input changed: %q", got)
	}
	got := tailLines("1\n2\n3\n4\n5", 2)
	if got != "… (3 earlier lines omitted)\n4\n5" {
		t.Errorf("tailLines truncation = %q", got)
	}
}

func TestFirstInt(t *testing.T) {
	a, b := 5, 9
	if got := firstInt(nil, &a, &b); got == nil || *got != 5 {
		t.Errorf("firstInt = %v, want 5", got)
	}
	if got := firstInt(nil, nil); got != nil {
		t.Errorf("firstInt(nil,nil) = %v, want nil", got)
	}
}
