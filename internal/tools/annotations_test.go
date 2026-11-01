package tools

import (
	"slices"
	"sort"
	"strings"
	"testing"
)

// Clients use these hints to decide what to auto-approve and what to confirm,
// and the connector directory expects them. Every tool needs a title and
// explicit safety hints; an unlisted new tool must fail here, not ship bare.

func TestEveryToolHasATitleAndSafetyHints(t *testing.T) {
	for _, tool := range listedTools(t) {
		a := tool.Annotations
		if a == nil {
			t.Errorf("%s has no annotations", tool.Name)
			continue
		}
		if tool.Title == "" || a.Title != tool.Title {
			t.Errorf("%s: tool title %q and annotation title %q must both be set and equal", tool.Name, tool.Title, a.Title)
		}
		// openWorldHint defaults to true when absent, so a closed-world tool
		// must say false out loud.
		if a.OpenWorldHint == nil {
			t.Errorf("%s leaves openWorldHint unset, which a client reads as true", tool.Name)
		}
		switch {
		case a.ReadOnlyHint && (a.DestructiveHint != nil || a.IdempotentHint):
			t.Errorf("%s is read-only, so destructive/idempotent hints are meaningless", tool.Name)
		case !a.ReadOnlyHint && a.DestructiveHint == nil:
			t.Errorf("%s changes state but leaves destructiveHint unset, which a client reads as true", tool.Name)
		}
	}
}

func TestNoHintRowForAToolThatDoesNotExist(t *testing.T) {
	registered := map[string]bool{}
	for _, tool := range listedTools(t) {
		registered[tool.Name] = true
	}
	for name := range hints {
		if !registered[name] {
			t.Errorf("hints has a row for %q, which is not a registered tool", name)
		}
	}
}

// The destructive set is the review surface: it is what a client should ask
// before running. Pinned by name so widening or narrowing it is a deliberate edit.
func TestDestructiveToolsArePinned(t *testing.T) {
	want := []string{
		"avd_snapshot", "clear_app_data", "clear_device_lock", "clear_logcat",
		"gradle_build", "pull_file", "push_file", "revoke_permission",
		"run_instrumented_tests", "set_device_lock", "shutdown_emulator",
		"uninstall_app",
	}
	var got []string
	for _, tool := range listedTools(t) {
		a := tool.Annotations
		if a != nil && !a.ReadOnlyHint && a.DestructiveHint != nil && *a.DestructiveHint {
			got = append(got, tool.Name)
		}
	}
	sort.Strings(got)
	if !slices.Equal(got, want) {
		t.Errorf("destructive tools changed:\n got %v\nwant %v", got, want)
	}
}

// A name that says it mutates must not be advertised as read-only.
func TestMutatingNamesAreNeverReadOnly(t *testing.T) {
	for _, tool := range listedTools(t) {
		for _, p := range []string{"set_", "clear_", "install_", "uninstall_", "push_", "pull_", "shutdown_", "grant_", "revoke_", "start_", "boot_", "launch_", "tap", "swipe", "input_"} {
			if strings.HasPrefix(tool.Name, p) && tool.Annotations != nil && tool.Annotations.ReadOnlyHint {
				t.Errorf("%s starts with %q but is advertised as read-only", tool.Name, p)
			}
		}
	}
}

func TestTitleFor(t *testing.T) {
	for name, want := range map[string]string{
		"tap":             "Tap",
		"tap_on_text":     "Tap on text",
		"describe_ui":     "Describe UI",
		"adb_reverse":     "ADB reverse",
		"avd_snapshot":    "AVD snapshot",
		"install_app":     "Install app",
		"send_sms":        "Send SMS",
		"enter_pin":       "Enter PIN",
		"open_url":        "Open URL",
		"get_app_details": "Get app details",
	} {
		if got := titleFor(name); got != want {
			t.Errorf("titleFor(%q) = %q, want %q", name, got, want)
		}
	}
}
