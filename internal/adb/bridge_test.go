package adb

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

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
	wantArgv(t, f.calls[1], []string{"shell", "am", "broadcast", "-a", BridgeClickAction, "-e", "text", "Chrome", "-e", "partial", "true"})
	wantArgv(t, f.calls[2], []string{"shell", "logcat", "-d", "-s", "AdbMcpBridge:I"})
}

func TestAccessibilityClickResourceID(t *testing.T) {
	ctx := context.Background()
	c, f := newFake(`I AdbMcpBridge: {"ok":true,"matched_resource_id":"com.example:id/submit","clickable":true,"action_result":true}`)

	if _, err := c.AccessibilityClick(ctx, "submit", "", false); err != nil {
		t.Fatalf("AccessibilityClick: %v", err)
	}
	wantArgv(t, f.calls[1], []string{"shell", "am", "broadcast", "-a", BridgeClickAction, "-e", "resource_id", "submit", "-e", "partial", "false"})
}

func TestAccessibilityClickNoResponse(t *testing.T) {
	c, _ := newFake("--------- beginning of main")
	if _, err := c.AccessibilityClick(context.Background(), "", "Missing", true); err == nil {
		t.Fatal("expected an error when the bridge never logs a result line")
	}
}
