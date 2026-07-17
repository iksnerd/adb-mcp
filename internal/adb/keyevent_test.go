package adb

import "testing"

func TestResolveKey(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"enter", 66},
		{"ENTER", 66},
		{" Back ", 4},
		{"home", 3},
		{"esc", 111},
		{"escape", 111},
		{"del", 67},
		{"app_switch", 187},
		{"66", 66},   // raw numeric passthrough
		{"999", 999}, // arbitrary keycode
	}
	for _, c := range cases {
		got, err := ResolveKey(c.in)
		if err != nil {
			t.Errorf("ResolveKey(%q) error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ResolveKey(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestResolveKeyErrors(t *testing.T) {
	for _, in := range []string{"", "  ", "boguskey"} {
		if _, err := ResolveKey(in); err == nil {
			t.Errorf("ResolveKey(%q) expected error, got nil", in)
		}
	}
}

func TestResolveCombo(t *testing.T) {
	cases := []struct {
		in   string
		want []int
	}{
		{"select_all", []int{113, 29}}, // ctrl, a
		{"SELECT_ALL", []int{113, 29}}, // case-insensitive
		{" copy ", []int{113, 31}},     // ctrl, c
		{"paste", []int{113, 50}},      // ctrl, v
		{"redo", []int{113, 59, 54}},   // ctrl, shift, z
	}
	for _, c := range cases {
		got, err := ResolveCombo(c.in)
		if err != nil {
			t.Errorf("ResolveCombo(%q) error: %v", c.in, err)
			continue
		}
		if len(got) != len(c.want) {
			t.Errorf("ResolveCombo(%q) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("ResolveCombo(%q) = %v, want %v", c.in, got, c.want)
				break
			}
		}
	}
}

func TestResolveComboErrors(t *testing.T) {
	for _, in := range []string{"", "boguspreset", "ctrl+a"} {
		if _, err := ResolveCombo(in); err == nil {
			t.Errorf("ResolveCombo(%q) expected error, got nil", in)
		}
	}
}
