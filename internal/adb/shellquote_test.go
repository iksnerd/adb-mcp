package adb

import (
	"context"
	"testing"
)

// shellHostile values are ones the device shell would split, truncate or
// expand if sent bare: `adb shell` re-parses its args on the device.
var shellHostile = []string{"Sign in", "a&b", "a;b", "it's", "pa$$word", "a b&c;d'e$f"}

func TestAccessibilityClickQuotesSelectors(t *testing.T) {
	for _, v := range shellHostile {
		c, f := newFake(`I AdbMcpBridge: {"ok":true}`)
		if _, err := c.AccessibilityClick(context.Background(), v, v, true); err != nil {
			t.Fatal(err)
		}
		wantArgv(t, f.calls[1], []string{"shell", "am", "broadcast", "-a", BridgeClickAction,
			"-e", "resource_id", escapeInputText(v), "-e", "text", escapeInputText(v), "-e", "partial", "true"})
	}
}

func TestDeviceLockQuotesCredentials(t *testing.T) {
	ctx := context.Background()
	for _, v := range shellHostile {
		c, f := newFake("")
		if err := c.SetDeviceLock(ctx, "password", v, v); err != nil {
			t.Fatal(err)
		}
		wantArgv(t, f.last(), []string{"shell", "locksettings", "set-password", "--old", escapeInputText(v), escapeInputText(v)})

		c, f = newFake("")
		if err := c.ClearDeviceLock(ctx, v); err != nil {
			t.Fatal(err)
		}
		wantArgv(t, f.last(), []string{"shell", "locksettings", "clear", "--old", escapeInputText(v)})
	}
}

func TestPackageShellCallsQuoteArgs(t *testing.T) {
	ctx := context.Background()
	for _, v := range shellHostile {
		q := escapeInputText(v)
		c, f := newFake("")
		if _, err := c.OpenURL(ctx, "https://x.test/?a=1&b=2;c "+v, v); err != nil {
			t.Fatal(err)
		}
		wantArgv(t, f.last(), []string{"shell", "am", "start", "-a", "android.intent.action.VIEW",
			"-d", escapeInputText("https://x.test/?a=1&b=2;c " + v), "-p", q})

		c, f = newFake("")
		_ = c.StopApp(ctx, v)
		wantArgv(t, f.last(), []string{"shell", "am", "force-stop", q})

		c, f = newFake("")
		_, _ = c.ClearAppData(ctx, v)
		wantArgv(t, f.last(), []string{"shell", "pm", "clear", q})

		c, f = newFake("")
		_ = c.ReloadApp(ctx, v)
		wantArgv(t, f.last(), []string{"shell", "am", "broadcast", "-a", escapeInputText(v + ".RELOAD_APP_ACTION")})

		c, f = newFake("")
		_, _ = c.LaunchApp(ctx, v)
		wantArgv(t, f.last(), []string{"shell", "monkey", "-p", q, "-v", "-c", "android.intent.category.LAUNCHER", "1"})

		c, f = newFake("")
		_, _ = c.RegisteredURLSchemes(ctx, v)
		wantArgv(t, f.last(), []string{"shell", "dumpsys", "package", q})

		c, f = newFake("")
		_, _ = c.GetAppDetails(ctx, v)
		wantArgv(t, f.calls[0], []string{"shell", "dumpsys", "package", q})
	}
}
