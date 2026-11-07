package tools

import (
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Every tool advertises a title and MCP safety hints, so a client can
// auto-approve what only observes and stop to ask before what destroys.
//
// A hint describes the tool's own effect, not what an app might do with the
// input: tap and input_text are "additive" even though a tap can land on a
// Delete button, because the tool sends an input event and removes nothing.
// What a client should ask about is the destructive set below, and it is
// pinned by name in annotations_test.go so changing it is a deliberate edit.
//
//   - observe:  reads state and changes nothing.
//   - change:   changes device, app or host state but deletes or overwrites
//     nothing that could not be put back by running it again.
//   - destroy:  can delete or overwrite data, end a running emulator, or
//     change a security setting.
//
// idempotent: calling again with the same arguments adds no further effect.
// network: the tool may reach beyond the device and this machine. Gradle
// resolves plugins and dependencies over the network and runs the project's own
// build scripts; open_url and connect_wireless reach a URL or a peer the caller
// names. Everything else acts on the attached device or emulator only, so it
// must say openWorldHint=false out loud (the default is true).
type effect int

const (
	observed effect = iota
	changed
	destroyed
)

type hint struct {
	effect effect
	idem   bool
	web    bool
}

func observe() hint { return hint{effect: observed} }
func change() hint  { return hint{effect: changed} }
func destroy() hint { return hint{effect: destroyed} }

func (h hint) idempotent() hint { h.idem = true; return h }
func (h hint) network() hint    { h.web = true; return h }

var hints = map[string]hint{
	// --- Emulator / device management ---
	"list_avds":         observe(),
	"boot_emulator":     destroy(), // wipe_data=true factory-resets the AVD
	"list_devices":      observe(),
	"wait_for_boot":     observe(),
	"shutdown_emulator": destroy().idempotent(),
	"avd_snapshot":      destroy(), // save|load|delete|list in one tool; load and delete overwrite state
	"connect_wireless":  change().network(),
	"doctor":            observe(),

	// --- Observe ---
	"screenshot":    observe(),
	"describe_ui":   observe(),
	"render_stats":  change(), // reset=true zeroes the gfxinfo frame counters
	"app_state":     observe(),
	"wait":          observe(),
	"wait_for_text": change(), // scroll=true swipes the screen while polling

	// --- Interact ---
	"tap":             change(),
	"tap_on_text":     change(),
	"tap_element":     change(),
	"long_press":      change(),
	"swipe":           change(),
	"drag":            change(),
	"input_text":      change(),
	"input_key_combo": change(),
	"press_key":       change(),
	"rotate_screen":   change(), // cycles orientation on every call, so a repeat is not a no-op
	"run_sequence":    change(),
	"open_url":        change().network(),

	// --- Lock / biometrics ---
	"enter_pin":              change(),
	"prefer_pin":             change(),
	"set_device_lock":        destroy(), // replaces the lock screen, a security setting
	"clear_device_lock":      destroy(),
	"is_device_secure":       observe(),
	"has_biometric_enrolled": observe(),
	"fingerprint_touch":      change(),
	"finger_remove":          change().idempotent(),

	// --- Apps ---
	"launch_app":        change(),
	"launch_dev_client": change(),
	"reload_app":        change(),
	"open_dev_menu":     change(),
	"stop_app":          change().idempotent(),
	"install_app":       change().idempotent(), // -r keeps app data
	"uninstall_app":     destroy().idempotent(),
	"clear_app_data":    destroy().idempotent(),
	"list_packages":     observe(),
	"get_app_details":   observe(),
	"grant_permission":  change().idempotent(),
	"revoke_permission": destroy().idempotent(),
	"adb_reverse":       change().idempotent(),
	"last_crash":        observe(),
	"push_file":         destroy(), // adb push overwrites the device path
	"pull_file":         destroy(), // adb pull overwrites the local path

	// --- Logs and capture ---
	"logcat":               observe(),
	"clear_logcat":         destroy().idempotent(),
	"start_logcat_capture": destroy(), // clear defaults true and runs logcat -c, as clear_logcat does
	"stop_logcat_capture":  change(),
	"start_screen_record":  change(),
	"stop_screen_record":   destroy(), // overwrites local_path, as pull_file does

	// --- Emulator environment ---
	"set_battery":    change().idempotent(),
	"set_dark_mode":  change().idempotent(),
	"set_location":   change().idempotent(),
	"set_sensor":     change().idempotent(),
	"set_status_bar": change().idempotent(),
	"cellular":       change().idempotent(),
	"stay_awake":     change().idempotent(),
	"send_sms":       change(),
	"phone_call":     change(),

	// --- Session defaults ---
	"session_set_defaults":   change().idempotent(),
	"session_show_defaults":  observe(),
	"session_clear_defaults": change().idempotent(),

	// --- Gradle ---
	"scaffold_android_project":  change(),            // never overwrites a non-empty directory
	"gradle_build":              destroy().network(), // task= is arbitrary: clean and publish are both reachable
	"build_and_run":             destroy().network(), // task=/args= are arbitrary, as for gradle_build
	"build_status":              observe(),
	"cancel_build":              destroy(),           // stops a running build
	"run_unit_tests":            destroy().network(), // task=/args= are arbitrary, as for gradle_build
	"run_instrumented_tests":    destroy().network(), // connectedAndroidTest uninstalls the app under test
	"get_coverage_report":       destroy().network(), // task=/args= are arbitrary, as for gradle_build
	"get_file_coverage":         destroy().network(), // task=/args= are arbitrary, as for gradle_build
	"list_gradle_projects":      observe().network(),
	"list_gradle_tasks":         destroy().network(), // task= is arbitrary (clean, --init-script), not only a listing task
	"list_gradle_variants":      destroy().network(), // task= is arbitrary, as for list_gradle_tasks
	"gradle_project_properties": observe().network(),
}

// annotationsFor builds the MCP annotations for a tool, or nil when the tool
// has no row (annotations_test.go fails the build for that).
func annotationsFor(name string) *mcp.ToolAnnotations {
	h, ok := hints[name]
	if !ok {
		return nil
	}
	web := h.web
	a := &mcp.ToolAnnotations{Title: titleFor(name), OpenWorldHint: &web}
	if h.effect == observed {
		a.ReadOnlyHint = true
		return a
	}
	d := h.effect == destroyed
	a.DestructiveHint = &d
	a.IdempotentHint = h.idem
	return a
}

var acronyms = map[string]string{
	"adb": "ADB", "avd": "AVD", "ui": "UI", "apk": "APK",
	"sms": "SMS", "pin": "PIN", "url": "URL",
}

// titleFor turns a tool name into a display title: "tap_on_text" becomes "Tap
// on text", with the acronyms above kept upper case.
func titleFor(name string) string {
	words := strings.Split(name, "_")
	for i, w := range words {
		switch {
		case acronyms[w] != "":
			words[i] = acronyms[w]
		case i == 0 && w != "":
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}
