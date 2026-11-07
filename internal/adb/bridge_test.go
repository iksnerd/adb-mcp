package adb

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseBridgeVersion(t *testing.T) {
	cases := []struct {
		name     string
		in       string
		wantCode int
		wantName string
	}{
		{"typical dumpsys", "Packages:\n  Package [com.iksnerd.adbmcp.bridge] (abc):\n    versionCode=2406 minSdk=26 targetSdk=36\n    versionName=0.24.6\n    flags=[ HAS_CODE ]\n", 2406, "0.24.6"},
		{"legacy bridge", "    versionCode=1 minSdk=26 targetSdk=36\n    versionName=1.0\n", 1, "1.0"},
		{"code only", "    versionCode=7 minSdk=26\n", 7, ""},
		{"first match wins", "versionCode=5\nversionName=a\nversionCode=9\nversionName=b\n", 5, "a"},
		{"empty", "", 0, ""},
		{"unrelated output", "Unable to find package: com.iksnerd.adbmcp.bridge", 0, ""},
		{"non numeric code", "versionCode=abc versionName=1", 0, "1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, name := parseBridgeVersion(tc.in)
			if code != tc.wantCode || name != tc.wantName {
				t.Errorf("parseBridgeVersion = (%d, %q), want (%d, %q)", code, name, tc.wantCode, tc.wantName)
			}
		})
	}
}

func TestClassifyBridgeVersion(t *testing.T) {
	cases := []struct {
		name      string
		installed int
		min       int
		want      BridgeVersionState
	}{
		{"below minimum", 1, 2406, BridgeVersionOlder},
		{"equal", 2406, 2406, BridgeVersionOK},
		{"above", 2500, 2406, BridgeVersionOK},
		{"legacy versionCode 1 with minimum 1", 1, 1, BridgeVersionOK},
		{"unknown", 0, 1, BridgeVersionUnknown},
		{"negative is unknown", -3, 1, BridgeVersionUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyBridgeVersion(tc.installed, tc.min); got != tc.want {
				t.Errorf("classifyBridgeVersion(%d, %d) = %v, want %v", tc.installed, tc.min, got, tc.want)
			}
		})
	}
}

func TestMinBridgeVersionCodeIsOneToday(t *testing.T) {
	if MinBridgeVersionCode != 1 {
		t.Errorf("MinBridgeVersionCode = %d; if you raised it, name the contract change that required it and update this test", MinBridgeVersionCode)
	}
}

func bridgeVersionFake(dumpsys string, dumpsysErr error) *Client {
	return &Client{Serial: "s", run: func(_ context.Context, args ...string) ([]byte, error) {
		switch {
		case len(args) > 2 && args[1] == "pm":
			return []byte("package:" + BridgePackage), nil
		case len(args) > 1 && args[1] == "dumpsys":
			return []byte(dumpsys), dumpsysErr
		}
		return []byte("null"), nil
	}}
}

func TestGetBridgeStatusReadsInstalledVersion(t *testing.T) {
	c := bridgeVersionFake("    versionCode=2406 minSdk=26\n    versionName=0.24.6\n", nil)
	st, err := c.GetBridgeStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.VersionCode != 2406 || st.VersionName != "0.24.6" {
		t.Errorf("version = (%d, %q), want (2406, 0.24.6)", st.VersionCode, st.VersionName)
	}
}

func TestGetBridgeStatusVersionFailureIsUnknownNotError(t *testing.T) {
	st, err := bridgeVersionFake("", errors.New("boom")).GetBridgeStatus(context.Background())
	if err != nil {
		t.Fatalf("a failed version read must not be an error: %v", err)
	}
	if !st.Installed || st.VersionCode != 0 {
		t.Errorf("status = %+v, want installed with unknown version", st)
	}
}

func TestBridgeVersionLine(t *testing.T) {
	cases := []struct {
		name    string
		st      BridgeStatus
		server  string
		min     int
		want    string
		warning bool
	}{
		{"ok", BridgeStatus{VersionCode: 2406, VersionName: "0.24.6"}, "0.24.6", 1, "bridge 0.24.6 (versionCode 2406), server 0.24.6", false},
		{"older", BridgeStatus{VersionCode: 1, VersionName: "1.0"}, "0.24.6", 2000, "adb-mcp bridge install", true},
		{"unknown", BridgeStatus{}, "0.24.6", 1, "bridge version unknown", false},
		{"no server version", BridgeStatus{VersionCode: 2406, VersionName: "0.24.6"}, "", 1, "bridge 0.24.6 (versionCode 2406)", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := bridgeVersionLine(tc.st, tc.server, tc.min)
			if !strings.Contains(got, tc.want) {
				t.Errorf("line = %q, want it to contain %q", got, tc.want)
			}
			if strings.Contains(got, "⚠") != tc.warning {
				t.Errorf("line = %q, warning = %v, want %v", got, strings.Contains(got, "⚠"), tc.warning)
			}
			if tc.name == "no server version" && strings.Contains(got, "server") {
				t.Errorf("line = %q, must not mention an unset server version", got)
			}
		})
	}
}

func TestInstallBridge(t *testing.T) {
	apk := filepath.Join(t.TempDir(), "bridge.apk")
	if err := os.WriteFile(apk, []byte("fake apk"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, f := newFake("Success")
	if _, err := c.InstallBridge(context.Background(), apk); err != nil {
		t.Fatalf("InstallBridge: %v", err)
	}
	wantArgv(t, f.last(), []string{"install", "-r", apk})
}

func TestEnableBridgeService(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name       string
		curList    string
		wantPutArg string
	}{
		{"no services enabled yet", "null", BridgeServiceComponent},
		{"other service already enabled", "com.other/.Service", "com.other/.Service:" + BridgeServiceComponent},
		{"already enabled — no-op put, still ensures master switch", BridgeServiceComponent, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, f := newFake(tc.curList)
			if err := c.EnableBridgeService(ctx); err != nil {
				t.Fatalf("EnableBridgeService: %v", err)
			}
			wantArgv(t, f.calls[0], []string{"shell", "settings", "get", "secure", "enabled_accessibility_services"})
			if tc.wantPutArg == "" {
				// Already enabled: only the get + the accessibility_enabled put.
				if len(f.calls) != 2 {
					t.Fatalf("calls = %v, want 2 (no redundant enabled_accessibility_services put)", f.calls)
				}
				wantArgv(t, f.calls[1], []string{"shell", "settings", "put", "secure", "accessibility_enabled", "1"})
				return
			}
			wantArgv(t, f.calls[1], []string{"shell", "settings", "put", "secure", "enabled_accessibility_services", tc.wantPutArg})
			wantArgv(t, f.calls[2], []string{"shell", "settings", "put", "secure", "accessibility_enabled", "1"})
		})
	}
}

func TestGetBridgeStatus(t *testing.T) {
	c, f := newFake("")
	f.reply = "package:" + BridgePackage
	status, err := c.GetBridgeStatus(context.Background())
	if err != nil {
		t.Fatalf("GetBridgeStatus: %v", err)
	}
	if !status.Installed {
		t.Errorf("Installed = false, want true (reply contained package: line)")
	}
	// second call (enabled_accessibility_services) reuses the same fake reply,
	// which doesn't contain the component, so Enabled should be false.
	if status.Enabled {
		t.Errorf("Enabled = true, want false")
	}
}

func TestAccessibilityClick(t *testing.T) {
	ctx := context.Background()
	c, f := newFake(`08-14 13:33:52.910  9805  9805 I AdbMcpBridge: {"ok":true,"matched_text":"Chrome","matched_resource_id":null,"clicked_own_node":true,"clickable":true,"action_result":true}`)

	result, err := c.AccessibilityClick(ctx, "", "Chrome", true)
	if err != nil {
		t.Fatalf("AccessibilityClick: %v", err)
	}
	if !result.OK || result.MatchedText != "Chrome" || !result.ActionResult {
		t.Errorf("result = %+v, want ok=true matched_text=Chrome action_result=true", result)
	}

	wantArgv(t, f.calls[0], []string{"shell", "logcat", "-c"})
	wantArgv(t, f.calls[1], []string{"shell", "am", "broadcast", "-a", BridgeClickAction, "-e", "text", "'Chrome'", "--ez", "partial", "true"})
	wantArgv(t, f.calls[2], []string{"shell", "logcat", "-d", "-s", "AdbMcpBridge:I"})
}

func TestAccessibilityClickResourceID(t *testing.T) {
	ctx := context.Background()
	c, f := newFake(`I AdbMcpBridge: {"ok":true,"matched_resource_id":"com.example:id/submit","clickable":true,"action_result":true}`)

	if _, err := c.AccessibilityClick(ctx, "submit", "", false); err != nil {
		t.Fatalf("AccessibilityClick: %v", err)
	}
	wantArgv(t, f.calls[1], []string{"shell", "am", "broadcast", "-a", BridgeClickAction, "-e", "resource_id", "'submit'", "--ez", "partial", "false"})
}

func TestAccessibilityClickNoResponse(t *testing.T) {
	c, _ := newFake("--------- beginning of main")
	if _, err := c.AccessibilityClick(context.Background(), "", "Missing", true); err == nil {
		t.Fatal("expected an error when the bridge never logs a result line")
	}
}
