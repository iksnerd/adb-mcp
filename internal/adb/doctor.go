package adb

import (
	"context"
	"fmt"
	"strings"

	"github.com/iksnerd/adb-mcp/internal/sdk"
)

// Doctor reports the health of the local Android tooling: SDK location, adb and
// emulator availability/versions, known AVDs, and attached devices — so a user
// can diagnose "nothing works" without leaving the MCP.
func Doctor(ctx context.Context) string {
	var b strings.Builder
	root := sdk.Root()
	if root == "" {
		b.WriteString("✗ Android SDK: could not resolve (set ANDROID_HOME or ANDROID_SDK_ROOT)\n")
	} else {
		b.WriteString(fmt.Sprintf("• Android SDK: %s\n", root))
	}

	adb := sdk.AdbPath()
	if out, err := runAdb(ctx, "", "version"); err == nil {
		first := strings.SplitN(strings.TrimSpace(out), "\n", 2)[0]
		b.WriteString(fmt.Sprintf("✓ adb: %s (%s)\n", first, adb))
	} else {
		b.WriteString(fmt.Sprintf("✗ adb: not runnable at %s (%v)\n", adb, err))
	}

	if avds, err := ListAVDs(ctx); err == nil {
		if len(avds) == 0 {
			b.WriteString("⚠ emulator: no AVDs found — create one in Android Studio's Device Manager\n")
		} else {
			b.WriteString(fmt.Sprintf("✓ emulator: %d AVD(s): %s\n", len(avds), strings.Join(avds, ", ")))
		}
	} else {
		b.WriteString(fmt.Sprintf("✗ emulator: not runnable (%v)\n", err))
	}

	var ready []Device
	if devices, err := ListDevices(ctx); err == nil {
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
			b.WriteString(fmt.Sprintf("✓ devices: %s\n", strings.Join(parts, ", ")))
		}
	} else {
		b.WriteString(fmt.Sprintf("✗ devices: %v\n", err))
	}

	// Accessibility bridge (EXPERIMENTAL, see bridge.go): only meaningful per
	// device, and only worth checking once a device is actually reachable.
	for _, d := range ready {
		status, err := New(d.Serial).GetBridgeStatus(ctx)
		switch {
		case err != nil:
			b.WriteString(fmt.Sprintf("⚠ accessibility bridge (%s): could not check (%v)\n", d.Serial, err))
		case status.Installed && status.Enabled:
			b.WriteString(fmt.Sprintf("✓ accessibility bridge (%s): installed & enabled — tap_on_text/tap_element via_accessibility=true is usable\n", d.Serial))
		case status.Installed:
			b.WriteString(fmt.Sprintf("⚠ accessibility bridge (%s): installed but not enabled — run `adb-mcp bridge install` again, or check enabled_accessibility_services\n", d.Serial))
		default:
			b.WriteString(fmt.Sprintf("• accessibility bridge (%s): not installed (EXPERIMENTAL — run `adb-mcp bridge install` to enable tap_on_text/tap_element via_accessibility=true)\n", d.Serial))
		}
	}

	return strings.TrimRight(b.String(), "\n")
}
