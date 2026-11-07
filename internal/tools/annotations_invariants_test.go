package tools

import (
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func toolProps(tool *mcp.Tool) []string {
	schema, _ := tool.InputSchema.(map[string]any)
	props, _ := schema["properties"].(map[string]any)
	var names []string
	for n := range props {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// A parameter that reaches beyond observing makes the tool more than read-only,
// whatever its name says. These are tested by reason (the input property), so a
// new tool cannot slip a mutating switch behind an observe() row.
var mutatingProps = []string{"task", "args", "scroll", "reset", "clear", "wipe_data", "remove", "overwrite"}

// readOnlyExceptions: reviewed tool+property pairs that may stay read-only.
// Add an entry only with a reason; none are needed today.
var readOnlyExceptions = map[string]string{}

func TestReadOnlyToolsHaveNoMutatingInputs(t *testing.T) {
	for _, tool := range listedTools(t) {
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			continue
		}
		for _, p := range toolProps(tool) {
			if slices.Contains(mutatingProps, p) {
				if _, ok := readOnlyExceptions[tool.Name+"."+p]; !ok {
					t.Errorf("%s is read-only but takes %q, which can mutate state or run arbitrary work", tool.Name, p)
				}
			}
		}
		for _, pre := range []string{"set_", "start_", "stop_"} {
			if strings.HasPrefix(tool.Name, pre) {
				if _, ok := readOnlyExceptions[tool.Name]; !ok {
					t.Errorf("%s starts with %q but is read-only", tool.Name, pre)
				}
			}
		}
	}
}

// destructiveProps delete or overwrite data. local_path is included for tools
// whose local_path is a write target (pull_file, stop_screen_record);
// push_file only reads it but is destructive for the device path it overwrites.
var destructiveProps = []string{"clear", "wipe_data", "remove", "overwrite", "local_path"}

// notDestructiveExceptions: reviewed tool+property pairs allowed below destroy.
var notDestructiveExceptions = map[string]string{
	"adb_reverse.remove": "removes a port forward, which running adb_reverse again recreates; no data is lost",
}

func TestToolsThatDeleteOrOverwriteAreDestructive(t *testing.T) {
	for _, tool := range listedTools(t) {
		a := tool.Annotations
		if a == nil {
			continue
		}
		if a.DestructiveHint != nil && *a.DestructiveHint {
			continue
		}
		for _, p := range toolProps(tool) {
			if !slices.Contains(destructiveProps, p) {
				continue
			}
			if _, ok := notDestructiveExceptions[tool.Name+"."+p]; !ok {
				t.Errorf("%s takes %q (deletes or overwrites) but is not destructive", tool.Name, p)
			}
		}
	}
}

// Anything that forwards a caller-chosen Gradle task or args can reach clean or
// publish, so it must not be advertised as less than destructive.
func TestGradleTaskTakingToolsAreDestructive(t *testing.T) {
	for _, tool := range listedTools(t) {
		props := toolProps(tool)
		if !slices.Contains(props, "task") && !slices.Contains(props, "args") {
			continue
		}
		a := tool.Annotations
		if a == nil || a.ReadOnlyHint || a.DestructiveHint == nil || !*a.DestructiveHint {
			t.Errorf("%s forwards a caller-chosen task/args but is not destructive", tool.Name)
		}
	}
}

// Idempotent means a repeat call adds no effect. Pinned by name so a new
// .idempotent() is a conscious change (rotate_screen cycles, so it is not here).
func TestIdempotentToolsArePinned(t *testing.T) {
	want := []string{
		"adb_reverse", "cellular", "clear_app_data", "clear_logcat",
		"finger_remove", "grant_permission", "install_app", "revoke_permission",
		"session_clear_defaults", "session_set_defaults", "set_battery",
		"set_dark_mode", "set_location", "set_sensor", "set_status_bar",
		"shutdown_emulator", "stay_awake", "stop_app", "uninstall_app",
	}
	var got []string
	for _, tool := range listedTools(t) {
		if tool.Annotations != nil && tool.Annotations.IdempotentHint {
			got = append(got, tool.Name)
		}
	}
	sort.Strings(got)
	if !slices.Equal(got, want) {
		t.Errorf("idempotent tools changed:\n got %v\nwant %v", got, want)
	}
}
