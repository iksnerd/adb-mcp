package adb

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Accessibility bridge constants. The bridge is a small companion
// AccessibilityService APK (see repo-root bridge/) that adb-mcp installs and
// enables once per device so tap_on_text/tap_element can dispatch a real
// AccessibilityNodeInfo.performAction(ACTION_CLICK) — the only mechanism
// that reaches some native views (Compose/RN NativeTabs bars, some
// overlays) a raw coordinate `input tap` doesn't. EXPERIMENTAL: the
// mechanism is verified live, but the specific "coordinate tap no-ops,
// accessibility click succeeds" differential from the original field
// report hasn't been reproduced against that reporter's app.
const (
	BridgePackage           = "com.iksnerd.adbmcp.bridge"
	BridgeServiceComponent  = BridgePackage + "/.BridgeAccessibilityService"
	BridgeClickAction       = "com.iksnerd.adbmcp.bridge.ACTION_CLICK"
	bridgeLogTag            = "AdbMcpBridge"
	bridgeClickPollAttempts = 5
	bridgeClickPollInterval = 150 * time.Millisecond
)

var bridgeLogLineRe = regexp.MustCompile(bridgeLogTag + `:\s*(\{.*\})`)

// InstallBridge installs the accessibility bridge APK. Thin wrapper over
// InstallApp, kept as its own method so bridge-specific pre/post steps have
// a home without touching call sites.
func (c *Client) InstallBridge(ctx context.Context, apkPath string) (string, error) {
	return c.InstallApp(ctx, apkPath)
}

// EnableBridgeService turns on the bridge's AccessibilityService via
// `settings put secure enabled_accessibility_services`, preserving any other
// already-enabled services in that colon-separated list rather than
// clobbering it. A no-op if the bridge is already enabled.
func (c *Client) EnableBridgeService(ctx context.Context) error {
	cur, err := c.adb(ctx, "shell", "settings", "get", "secure", "enabled_accessibility_services")
	if err != nil {
		return fmt.Errorf("reading enabled_accessibility_services: %w", err)
	}
	cur = strings.TrimSpace(cur)
	if cur == "null" {
		cur = ""
	}
	if bridgeServiceListed(cur) {
		// Already enabled; still make sure the master switch is on.
	} else {
		next := BridgeServiceComponent
		if cur != "" {
			next = cur + ":" + BridgeServiceComponent
		}
		if _, err := c.adb(ctx, "shell", "settings", "put", "secure", "enabled_accessibility_services", next); err != nil {
			return fmt.Errorf("enabling %s: %w", BridgeServiceComponent, err)
		}
	}
	if _, err := c.adb(ctx, "shell", "settings", "put", "secure", "accessibility_enabled", "1"); err != nil {
		return fmt.Errorf("setting accessibility_enabled: %w", err)
	}
	return nil
}

func bridgeServiceListed(enabledList string) bool {
	for entry := range strings.SplitSeq(enabledList, ":") {
		if entry == BridgeServiceComponent {
			return true
		}
	}
	return false
}

// BridgeStatus reports whether the accessibility bridge is installed and
// enabled on the device — the pre-flight check for AccessibilityClick and a
// doctor diagnostic line.
type BridgeStatus struct {
	Installed bool
	Enabled   bool
	// VersionCode and VersionName are the installed bridge's, or 0 and ""
	// when they could not be read (not installed, or dumpsys output we do not
	// recognise). Unknown is never an error.
	VersionCode int
	VersionName string
}

// MinBridgeVersionCode is the oldest installed bridge versionCode this server
// still works with. Bump it in the same commit that changes the Go<->bridge
// contract: the broadcast action, the logcat tag, the extras, or the JSON keys
// of the result line. The bridge's versionCode is major*10000 + minor*100 +
// patch of the VERSION file (0.24.6 -> 2406); see bridge/app/build.gradle.kts.
const MinBridgeVersionCode = 1

// ServerVersion is the adb-mcp version shown next to the bridge's in doctor
// output. The main package sets it at startup; empty leaves it out.
var ServerVersion string

// BridgeVersionState classifies an installed bridge against a minimum.
type BridgeVersionState int

const (
	BridgeVersionUnknown BridgeVersionState = iota
	BridgeVersionOK
	BridgeVersionOlder
)

// classifyBridgeVersion compares an installed versionCode to a minimum. A
// code that is not positive means the version could not be read.
func classifyBridgeVersion(installed, minimum int) BridgeVersionState {
	switch {
	case installed <= 0:
		return BridgeVersionUnknown
	case installed < minimum:
		return BridgeVersionOlder
	default:
		return BridgeVersionOK
	}
}

var (
	bridgeVersionCodeRe = regexp.MustCompile(`versionCode=(\d+)`)
	bridgeVersionNameRe = regexp.MustCompile(`versionName=(\S+)`)
)

// parseBridgeVersion pulls the first versionCode and versionName out of
// `dumpsys package` output. The layout is not verified against real devices,
// so anything unrecognised yields zero values rather than an error.
func parseBridgeVersion(dumpsys string) (code int, name string) {
	if m := bridgeVersionCodeRe.FindStringSubmatch(dumpsys); m != nil {
		code, _ = strconv.Atoi(m[1])
	}
	if m := bridgeVersionNameRe.FindStringSubmatch(dumpsys); m != nil {
		name = m[1]
	}
	return code, name
}

// bridgeVersionLine is the doctor's version line for an installed bridge.
// serverVersion may be empty, in which case it is left out. Information only.
func bridgeVersionLine(s BridgeStatus, serverVersion string, minimum int) string {
	switch classifyBridgeVersion(s.VersionCode, minimum) {
	case BridgeVersionUnknown:
		return "bridge version unknown"
	case BridgeVersionOlder:
		return fmt.Sprintf("⚠ bridge %s (versionCode %d) is older than this server needs (versionCode %d): run `adb-mcp bridge install`", s.VersionName, s.VersionCode, minimum)
	}
	line := fmt.Sprintf("bridge %s (versionCode %d)", s.VersionName, s.VersionCode)
	if serverVersion != "" {
		line += ", server " + serverVersion
	}
	return line
}

// GetBridgeStatus reads the bridge's install/enable state and, when
// installed, its version.
func (c *Client) GetBridgeStatus(ctx context.Context) (BridgeStatus, error) {
	var s BridgeStatus
	pkgs, err := c.ListPackages(ctx, BridgePackage)
	if err != nil {
		return s, err
	}
	s.Installed = len(pkgs) > 0
	if s.Installed {
		if out, err := c.adb(ctx, "shell", "dumpsys", "package", BridgePackage); err == nil {
			s.VersionCode, s.VersionName = parseBridgeVersion(out)
		}
	}

	enabledList, err := c.adb(ctx, "shell", "settings", "get", "secure", "enabled_accessibility_services")
	if err != nil {
		return s, err
	}
	s.Enabled = bridgeServiceListed(strings.TrimSpace(enabledList))
	return s, nil
}

// BridgeClickResult mirrors the JSON line BridgeAccessibilityService logs
// after handling a click broadcast (bridge/app/src/.../BridgeAccessibilityService.kt).
type BridgeClickResult struct {
	OK                bool   `json:"ok"`
	Reason            string `json:"reason,omitempty"`
	MatchedText       string `json:"matched_text,omitempty"`
	MatchedResourceID string `json:"matched_resource_id,omitempty"`
	ClickedOwnNode    bool   `json:"clicked_own_node,omitempty"`
	Clickable         bool   `json:"clickable,omitempty"`
	ActionResult      bool   `json:"action_result,omitempty"`
}

// AccessibilityClick dispatches a real accessibility click through the
// bridge: clears logcat, broadcasts ACTION_CLICK with the given selector,
// then polls logcat for the bridge's JSON result line. Exactly one of
// resourceID/text should normally be set (mirrors tap_element vs
// tap_on_text); if both are set the bridge matches by resource_id first.
func (c *Client) AccessibilityClick(ctx context.Context, resourceID, text string, partial bool) (BridgeClickResult, error) {
	var result BridgeClickResult

	if _, err := c.adb(ctx, "shell", "logcat", "-c"); err != nil {
		return result, fmt.Errorf("clearing logcat before accessibility click: %w", err)
	}

	args := []string{"shell", "am", "broadcast", "-a", BridgeClickAction}
	if resourceID != "" {
		args = append(args, "-e", "resource_id", escapeInputText(resourceID))
	}
	if text != "" {
		args = append(args, "-e", "text", escapeInputText(text))
	}
	args = append(args, "--ez", "partial", strconv.FormatBool(partial))
	if _, err := c.adb(ctx, args...); err != nil {
		return result, fmt.Errorf("broadcasting accessibility click: %w", err)
	}

	var line string
	for attempt := range bridgeClickPollAttempts {
		out, err := c.adb(ctx, "shell", "logcat", "-d", "-s", bridgeLogTag+":I")
		if err == nil {
			if m := bridgeLogLineRe.FindAllStringSubmatch(out, -1); len(m) > 0 {
				line = m[len(m)-1][1] // most recent match
				break
			}
		}
		if attempt < bridgeClickPollAttempts-1 {
			time.Sleep(bridgeClickPollInterval)
		}
	}
	if line == "" {
		return result, fmt.Errorf("no response from the accessibility bridge — is it installed and enabled? run `adb-mcp bridge install`")
	}
	if err := json.Unmarshal([]byte(line), &result); err != nil {
		return result, fmt.Errorf("parsing bridge result %q: %w", line, err)
	}
	return result, nil
}
