package adb

import (
	"context"
	"fmt"
	"strings"

	"github.com/iksnerd/adb-mcp/internal/concurrent"
	"github.com/iksnerd/adb-mcp/internal/sdk"
)

// Doctor reports the health of the local Android tooling: SDK location, adb and
// emulator availability/versions, known AVDs, attached devices, and the
// accessibility bridge per ready device — so a user can diagnose "nothing
// works" without leaving the MCP.
//
// Device-side only. The host build toolchain (JDK, system Gradle) is
// gradle.ToolchainReport's job; the doctor tool composes the two, so this
// package never has to know what a JDK is.
func Doctor(ctx context.Context) string {
	var b strings.Builder
	root := sdk.Root()
	switch {
	case root == "":
		b.WriteString("✗ Android SDK: could not resolve (set ANDROID_HOME, or pass --sdk)\n")
	case !sdk.IsSDKRoot(root):
		// Root() falls back to the conventional install path, so a nonexistent
		// SDK still yields a plausible-looking path. Say it isn't there, or the
		// Gradle tools fail later with "SDK location not found" and this line
		// reads like confirmation that the SDK is fine.
		fmt.Fprintf(&b, "✗ Android SDK: %s has no platform-tools/ — not an SDK install (set ANDROID_HOME, or pass --sdk)\n", root)
	default:
		fmt.Fprintf(&b, "• Android SDK: %s\n", root)
	}

	// adb version, AVDs, and attached devices are three independent probes —
	// run them concurrently and join before writing, so the output order below
	// stays fixed regardless of which probe actually finishes first.
	var versionOut string
	var versionErr error
	var avds []string
	var avdsErr error
	var devices []Device
	var devicesErr error
	concurrent.RunAll(
		func() { versionOut, versionErr = runAdb(ctx, "", "version") },
		func() { avds, avdsErr = ListAVDs(ctx) },
		func() { devices, devicesErr = ListDevices(ctx) },
	)

	adb := sdk.AdbPath()
	if versionErr == nil {
		first, _, _ := strings.Cut(strings.TrimSpace(versionOut), "\n")
		fmt.Fprintf(&b, "✓ adb: %s (%s)\n", first, adb)
	} else {
		fmt.Fprintf(&b, "✗ adb: not runnable at %s (%v)\n", adb, versionErr)
	}

	if avdsErr == nil {
		if len(avds) == 0 {
			b.WriteString("⚠ emulator: no AVDs found — create one in Android Studio's Device Manager\n")
		} else {
			fmt.Fprintf(&b, "✓ emulator: %d AVD(s): %s\n", len(avds), strings.Join(avds, ", "))
		}
	} else {
		fmt.Fprintf(&b, "✗ emulator: not runnable (%v)\n", avdsErr)
	}

	var ready []Device
	if devicesErr == nil {
		if len(devices) == 0 {
			b.WriteString("• devices: none attached\n")
		} else {
			var parts []string
			for _, d := range devices {
				parts = append(parts, d.Serial+" ("+d.State+")")
				if d.State == "device" {
					ready = append(ready, d)
				}
			}
			fmt.Fprintf(&b, "✓ devices: %s\n", strings.Join(parts, ", "))
		}
	} else {
		fmt.Fprintf(&b, "✗ devices: %v\n", devicesErr)
	}

	// Accessibility bridge (EXPERIMENTAL, see bridge.go): only meaningful per
	// device, and only worth checking once a device is actually reachable. Each
	// device's check is an independent adb round-trip, so probe them all
	// concurrently and write results in `ready` order afterward.
	type bridgeResult struct {
		status BridgeStatus
		err    error
	}
	results := make([]bridgeResult, len(ready))
	concurrent.RunIndexed(len(ready), func(i int) {
		results[i].status, results[i].err = New(ready[i].Serial).GetBridgeStatus(ctx)
	})

	for i, d := range ready {
		status, err := results[i].status, results[i].err
		switch {
		case err != nil:
			fmt.Fprintf(&b, "⚠ accessibility bridge (%s): could not check (%v)\n", d.Serial, err)
		case status.Installed && status.Enabled:
			fmt.Fprintf(&b, "✓ accessibility bridge (%s): installed & enabled — tap_on_text/tap_element via_accessibility=true is usable\n", d.Serial)
		case status.Installed:
			fmt.Fprintf(&b, "⚠ accessibility bridge (%s): installed but not enabled — run `adb-mcp bridge install` again, or check enabled_accessibility_services\n", d.Serial)
		default:
			fmt.Fprintf(&b, "• accessibility bridge (%s): not installed (EXPERIMENTAL — run `adb-mcp bridge install` to enable tap_on_text/tap_element via_accessibility=true)\n", d.Serial)
		}
	}

	return strings.TrimRight(b.String(), "\n")
}
