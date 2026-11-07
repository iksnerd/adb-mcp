package adb

import (
	"strings"
	"testing"
)

func TestParseLogLineColonTag(t *testing.T) {
	line := "11-07 10:00:00.123  1234  5678 I Foo:Bar  : hello: world"
	prio, tag, ok := parseLogLine(line)
	if !ok || prio != "I" || tag != "Foo:Bar" {
		t.Fatalf("parseLogLine = %q %q %v, want I Foo:Bar", prio, tag, ok)
	}
	plain := "11-07 10:00:00.123  1234  5678 W ActivityManager: slow: very"
	if prio, tag, ok := parseLogLine(plain); !ok || prio != "W" || tag != "ActivityManager" {
		t.Fatalf("plain tag parsed as %q %q %v", prio, tag, ok)
	}
}

func TestLogFilterColonTag(t *testing.T) {
	raw := strings.Join([]string{
		"11-07 10:00:00.123  1234  5678 I Foo:Bar  : keep me",
		"11-07 10:00:00.124  1234  5678 I Foo      : drop me",
	}, "\n")
	got := LogFilter{Tags: []string{"Foo:Bar"}}.apply(raw)
	if !strings.Contains(got, "keep me") || strings.Contains(got, "drop me") {
		t.Fatalf("filter on a colon tag kept the wrong lines:\n%s", got)
	}
}
