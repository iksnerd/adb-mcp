package tools

import (
	"context"
	"strings"
	"testing"
)

// fakeAdb always attaches exactly one device, emulator-5554.
func TestListDevicesSerialFilter(t *testing.T) {
	run := func(t *testing.T, serial, pinned string) string {
		t.Helper()
		resetSessionDefaults(t)
		fakeAdb(t, "")
		if pinned != "" {
			sessionDefaults.mu.Lock()
			sessionDefaults.serial = pinned
			sessionDefaults.mu.Unlock()
		}
		res, err := listDevices(context.Background(), serialArg{Serial: serial})
		if err != nil {
			t.Fatalf("listDevices returned an error, want a normal result: %v", err)
		}
		return resultText(t, res)
	}
	t.Run("match", func(t *testing.T) {
		if got := run(t, "emulator-5554", ""); !strings.Contains(got, "emulator-5554") || strings.Contains(got, "No device") {
			t.Errorf("got %q, want the matching device", got)
		}
	})
	t.Run("trimmed match", func(t *testing.T) {
		if got := run(t, "  emulator-5554 ", ""); !strings.Contains(got, "emulator-5554") || strings.Contains(got, "No device") {
			t.Errorf("got %q, want the matching device", got)
		}
	})
	t.Run("no match", func(t *testing.T) {
		got := run(t, "ghost-1", "")
		if !strings.Contains(got, "No device with serial ghost-1 is attached.") || !strings.Contains(got, "Attached: emulator-5554") {
			t.Errorf("got %q, want the no-match message listing attached serials", got)
		}
	})
	t.Run("blank lists all", func(t *testing.T) {
		if got := run(t, "  ", ""); !strings.Contains(got, "emulator-5554") || strings.Contains(got, "No device") {
			t.Errorf("got %q, want the full list", got)
		}
	})
	t.Run("session default does not filter", func(t *testing.T) {
		got := run(t, "", "pinned-1")
		if !strings.Contains(got, "emulator-5554") || strings.Contains(got, "No device") {
			t.Errorf("got %q, want every device despite the pinned serial", got)
		}
	})
}
