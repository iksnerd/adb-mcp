package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// bridgeFake answers the bridge pre-flight, the click broadcast and the
// logcat result line, and serves a different UI hierarchy once the click
// broadcast has happened (changed=true) or the same one (changed=false).
func bridgeFake(t *testing.T, changed bool) {
	t.Helper()
	after := "Before"
	if changed {
		after = "After"
	}
	fakeAdb(t, `
dump() { printf "<?xml version='1.0'?><hierarchy><node text=\"$1\" clickable=\"true\" bounds=\"[0,0][10,10]\"/></hierarchy>"; }
case "$*" in
  *"list packages"*) echo package:com.iksnerd.adbmcp.bridge;;
  *enabled_accessibility_services*) echo com.iksnerd.adbmcp.bridge/.BridgeAccessibilityService;;
  *"am broadcast"*) touch "$ANDROID_HOME/clicked";;
  *"logcat -d"*) echo 'I AdbMcpBridge: {"ok":true,"matched_text":"Go","clickable":true,"action_result":true}';;
  *"cat /sdcard/window_dump.xml"*) if [ -f "$ANDROID_HOME/clicked" ]; then dump `+after+`; else dump Before; fi;;
esac`)
}

func resultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	return res.Content[0].(*mcp.TextContent).Text
}

// verify_change used to be dropped silently when via_accessibility=true: the
// handler returned before the change check ran.
func TestViaAccessibilityHonoursVerifyChange(t *testing.T) {
	yes := true
	for _, tc := range []struct {
		name    string
		changed bool
		verify  *bool
		want    string
		notWant string
	}{
		{"changed", true, &yes, "ui_changed: true", ""},
		{"unchanged", false, &yes, "ui_changed: false", ""},
		{"not requested", true, nil, "", "ui_changed"},
	} {
		t.Run("tap_on_text/"+tc.name, func(t *testing.T) {
			bridgeFake(t, tc.changed)
			res, err := tapOnText(context.Background(), tapTextArgs{Text: "Go", ViaAccessibility: &yes, VerifyChange: tc.verify})
			if err != nil {
				t.Fatal(err)
			}
			got := resultText(t, res)
			if tc.want != "" && !strings.Contains(got, tc.want) {
				t.Errorf("%q lacks %q", got, tc.want)
			}
			if tc.notWant != "" && strings.Contains(got, tc.notWant) {
				t.Errorf("%q should not mention %q", got, tc.notWant)
			}
		})
		t.Run("tap_element/"+tc.name, func(t *testing.T) {
			bridgeFake(t, tc.changed)
			res, err := tapElement(context.Background(), tapElementArgs{ResourceID: "go", ViaAccessibility: &yes, VerifyChange: tc.verify})
			if err != nil {
				t.Fatal(err)
			}
			got := resultText(t, res)
			if tc.want != "" && !strings.Contains(got, tc.want) {
				t.Errorf("%q lacks %q", got, tc.want)
			}
			if tc.notWant != "" && strings.Contains(got, tc.notWant) {
				t.Errorf("%q should not mention %q", got, tc.notWant)
			}
		})
	}
}
