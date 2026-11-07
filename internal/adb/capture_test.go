package adb

import (
	"strings"
	"testing"
)

// Pins the registry contract the tools layer relies on: sessions are keyed by
// the concrete serial (tools.resolve always passes ResolveSerial's result), so
// a second start for the same device is refused and stop finds the session.
func TestSessionRegistryKeyedBySerial(t *testing.T) {
	r := &sessionRegistry[recordSession]{}
	mk := func() (*recordSession, error) { return &recordSession{}, nil }

	if err := r.start("emulator-5554", "screen recording", mk); err != nil {
		t.Fatal(err)
	}
	err := r.start("emulator-5554", "screen recording", mk)
	if err == nil || !strings.Contains(err.Error(), "already running for emulator-5554") {
		t.Fatalf("expected already-running error, got %v", err)
	}
	if _, ok := r.take("emulator-5554"); !ok {
		t.Fatal("stop should find the session under the same serial")
	}
	if _, ok := r.take("emulator-5554"); ok {
		t.Fatal("session should be gone after take")
	}
}
