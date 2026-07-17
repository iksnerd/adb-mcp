package adb

import (
	"context"
	"fmt"
	"strings"

	"github.com/iksnerd/adb_mcp/internal/sdk"
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

	if devices, err := ListDevices(ctx); err == nil {
		if len(devices) == 0 {
			b.WriteString("• devices: none attached\n")
		} else {
			var parts []string
			for _, d := range devices {
				parts = append(parts, d.Serial+" ("+d.State+")")
			}
			b.WriteString(fmt.Sprintf("✓ devices: %s\n", strings.Join(parts, ", ")))
		}
	} else {
		b.WriteString(fmt.Sprintf("✗ devices: %v\n", err))
	}

	return strings.TrimRight(b.String(), "\n")
}
