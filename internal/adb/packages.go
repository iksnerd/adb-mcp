package adb

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// ListPackages lists installed package names, optionally filtered by substring.
func (c *Client) ListPackages(ctx context.Context, filter string) ([]string, error) {
	out, err := c.adb(ctx, "shell", "pm", "list", "packages")
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(strings.TrimSpace(filter))
	var pkgs []string
	for line := range strings.SplitSeq(out, "\n") {
		pkg := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "package:"))
		if pkg == "" {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(pkg), needle) {
			continue
		}
		pkgs = append(pkgs, pkg)
	}
	return pkgs, nil
}

// InstallApp installs (or reinstalls, -r) an APK from a local path. Some
// adb/device combinations report install failures as a "Failure [...]" line
// with a zero exit code, so the output is scanned too — a nil error means the
// APK really landed.
func (c *Client) InstallApp(ctx context.Context, apkPath string) (string, error) {
	if _, err := os.Stat(apkPath); err != nil {
		return "", fmt.Errorf("apk not found: %s", apkPath)
	}
	out, err := c.adb(ctx, "install", "-r", apkPath)
	if err == nil && strings.Contains(out, "Failure [") {
		return out, fmt.Errorf("adb install %s: %s", apkPath, strings.TrimSpace(out))
	}
	return out, err
}

// UninstallApp removes an app by package name.
func (c *Client) UninstallApp(ctx context.Context, pkg string) (string, error) {
	return c.adb(ctx, "uninstall", pkg)
}

// launchComponentRe pulls the resolved component out of monkey's verbose
// output line: "// Allowing start of Intent { ... cmp=<pkg>/<activity> ... }".
var launchComponentRe = regexp.MustCompile(`cmp=(\S+)`)

// LaunchApp starts an app's launcher activity via monkey and returns the
// resolved component on success. monkey exits non-zero and prints "No
// activities found to run, monkey aborted" when the package isn't installed or
// has no LAUNCHER activity; that raw arg-dump is unreadable, so surface a clear
// error instead. component may be "" if monkey didn't echo it.
func (c *Client) LaunchApp(ctx context.Context, pkg string) (component string, err error) {
	out, runErr := c.adb(ctx, "shell", "monkey", "-p", pkg, "-v",
		"-c", "android.intent.category.LAUNCHER", "1")
	if strings.Contains(out, "No activities found to run") || strings.Contains(out, "monkey aborted") {
		return "", fmt.Errorf("cannot launch %s: no launchable activity (is the package installed, and does it have a LAUNCHER activity? check list_packages / get_app_details)", pkg)
	}
	if runErr != nil {
		return "", runErr
	}
	if m := launchComponentRe.FindStringSubmatch(out); m != nil {
		component = m[1]
	}
	return component, nil
}

// StopApp force-stops an app.
func (c *Client) StopApp(ctx context.Context, pkg string) error {
	_, err := c.adb(ctx, "shell", "am", "force-stop", pkg)
	return err
}

// ReloadApp attempts to trigger a Metro/JS reload via the classic React
// Native dev-support broadcast receiver (<pkg>.RELOAD_APP_ACTION). This is
// best-effort: the receiver is only registered in debug builds of classic
// (non-bridgeless) RN architectures, so on newer RN/Expo dev clients this
// broadcast may be silently ignored with no error. When it doesn't visibly
// reload the app, fall back to OpenDevMenu + tapping "Reload".
func (c *Client) ReloadApp(ctx context.Context, pkg string) error {
	_, err := c.adb(ctx, "shell", "am", "broadcast", "-a", pkg+".RELOAD_APP_ACTION")
	return err
}

// OpenDevMenu opens the React Native dev menu (KEYCODE_MENU) on the
// foreground app — the standard adb way to reach a dev build's Reload/Debug
// JS Remotely/etc. options. From here, tap_on_text("Reload") (or another menu
// item) drives it.
func (c *Client) OpenDevMenu(ctx context.Context) error {
	return c.PressKey(ctx, 82)
}

// ClearAppData wipes an app's data/cache, returning it to a first-launch state.
func (c *Client) ClearAppData(ctx context.Context, pkg string) (string, error) {
	return c.adb(ctx, "shell", "pm", "clear", pkg)
}

// ExpoDevClientURL builds the deep link that points an installed Expo dev build
// straight at a Metro dev server, skipping the Dev Launcher's server-picker
// screen: "<scheme>://expo-development-client/?url=<url-encoded http URL>".
// scheme is the app's own URL scheme (from app.json "scheme"); host/port
// default to localhost:8081 (localhost works once adb_reverse tcp:8081 is set).
// Pure string building — no device — so it is unit-tested directly.
func ExpoDevClientURL(scheme, host string, port int) (string, error) {
	scheme = strings.TrimSpace(scheme)
	scheme = strings.TrimSuffix(scheme, "://")
	if scheme == "" {
		return "", fmt.Errorf("a dev-client scheme is required (your app.json \"scheme\", e.g. \"myapp\")")
	}
	if strings.TrimSpace(host) == "" {
		host = "localhost"
	}
	if port <= 0 {
		port = 8081
	}
	server := fmt.Sprintf("http://%s:%d", host, port)
	return fmt.Sprintf("%s://expo-development-client/?url=%s", scheme, url.QueryEscape(server)), nil
}

// OpenURL opens a URL or deep link via an ACTION_VIEW intent. When pkg is set
// the intent is targeted at that package.
func (c *Client) OpenURL(ctx context.Context, url, pkg string) (string, error) {
	args := []string{"shell", "am", "start", "-a", "android.intent.action.VIEW", "-d", url}
	if strings.TrimSpace(pkg) != "" {
		// Restrict the intent to a package with the -p option. A bare positional
		// argument would be parsed by `am` as the intent DATA URI, clobbering the
		// -d url above and silently opening the wrong thing.
		args = append(args, "-p", pkg)
	}
	return c.adb(ctx, args...)
}

// AppDetails is a compact summary of an installed package.
type AppDetails struct {
	Package          string `json:"package"`
	Installed        bool   `json:"installed"`
	VersionName      string `json:"version_name,omitempty"`
	VersionCode      string `json:"version_code,omitempty"`
	LauncherActivity string `json:"launcher_activity,omitempty"`
}

var (
	versionNameRe = regexp.MustCompile(`versionName=(\S+)`)
	versionCodeRe = regexp.MustCompile(`versionCode=(\d+)`)
)

// AppState is a runtime snapshot of an installed app: whether and where its
// process is running, and — for React Native / Expo dev builds — whether it is
// serving a live Metro bundle or its baked-in embedded one. Getting the last
// distinction wrong is the expensive failure: a dev client that silently fell
// back to its embedded bundle ignores every code edit, so taps and log reads run
// against stale code with no obvious signal. Two live processes for one package
// (a lingering old build + a fresh install) is the same class of trap — presses
// and log reads can hit different pids.
type AppState struct {
	Package        string   `json:"package"`
	Installed      bool     `json:"installed"`
	Running        bool     `json:"running"`
	PIDs           []int    `json:"pids,omitempty"`
	ProcessUptime  string   `json:"process_uptime,omitempty"` // wall-clock of the main process (ps ETIME)
	FirstInstall   string   `json:"first_install_time,omitempty"`
	LastUpdate     string   `json:"last_update_time,omitempty"`
	BundleSource   string   `json:"bundle_source"`             // metro | embedded | unknown | not-react-native
	BundleEvidence string   `json:"bundle_evidence,omitempty"` // the log line(s) the guess is based on
	Notes          []string `json:"notes,omitempty"`
}

var (
	firstInstallRe = regexp.MustCompile(`firstInstallTime=(.+)`)
	lastUpdateRe   = regexp.MustCompile(`lastUpdateTime=(.+)`)
)

// GetAppState assembles an AppState for pkg from several device probes: install
// state/times (dumpsys package), live pids (pidof), the main process's uptime
// (ps ETIME), and a Metro-vs-embedded bundle heuristic over the app's recent
// logcat. Every probe is best-effort — a failure annotates the result rather
// than failing the whole call, so a partial answer still beats none.
func (c *Client) GetAppState(ctx context.Context, pkg string) (AppState, error) {
	s := AppState{Package: pkg, BundleSource: "unknown"}

	if dump, err := c.adb(ctx, "shell", "dumpsys", "package", pkg); err == nil {
		if !strings.Contains(dump, "Unable to find package") && strings.Contains(dump, "Package [") {
			s.Installed = true
		}
		if m := firstInstallRe.FindStringSubmatch(dump); m != nil {
			s.FirstInstall = strings.TrimSpace(m[1])
		}
		if m := lastUpdateRe.FindStringSubmatch(dump); m != nil {
			s.LastUpdate = strings.TrimSpace(m[1])
		}
	}
	if !s.Installed {
		s.Notes = append(s.Notes, "package not installed (or dumpsys package couldn't read it)")
		return s, nil
	}

	if out, err := c.adb(ctx, "shell", "pidof", pkg); err == nil {
		s.PIDs = parsePIDs(out)
	}
	s.Running = len(s.PIDs) > 0
	if !s.Running {
		s.Notes = append(s.Notes, "not running — launch_app first; bundle source can't be determined for a stopped app")
		s.BundleSource = "n/a"
		return s, nil
	}
	if len(s.PIDs) > 1 {
		s.Notes = append(s.Notes, fmt.Sprintf("%d live processes for this package — taps and log reads may be hitting different ones; stop_app then launch_app for a clean single process", len(s.PIDs)))
	}
	if out, err := c.adb(ctx, "shell", "ps", "-o", "ETIME=", "-p", strconv.Itoa(s.PIDs[0])); err == nil {
		s.ProcessUptime = strings.TrimSpace(out)
	}

	// Bundle source: scan the app's recent logcat for dev-server / hot-reload
	// markers. --pid keeps it to this app's own lines.
	if logs, err := c.adb(ctx, "shell", "logcat", "-d", "-t", "4000", "--pid", strconv.Itoa(s.PIDs[0])); err == nil {
		s.BundleSource, s.BundleEvidence = classifyBundle(logs)
	}
	switch s.BundleSource {
	case "metro":
		s.Notes = append(s.Notes, "serving a live Metro bundle — code edits apply after a reload_app / dev-menu Reload")
	case "embedded":
		s.Notes = append(s.Notes, "running the EMBEDDED bundle — your JS edits are NOT in this process; if you expected Metro, run adb_reverse tcp:8081 and relaunch")
	case "not-react-native":
		s.BundleSource = "n/a"
	case "unknown":
		s.Notes = append(s.Notes, "couldn't tell Metro from embedded — no React-Native markers in recent logcat; clear_logcat, reload, and re-check, or it may be a native (non-RN) app")
	}
	return s, nil
}

// parsePIDs parses the space/newline-separated pid list from `pidof`.
func parsePIDs(out string) []int {
	var pids []int
	for f := range strings.FieldsSeq(out) {
		if n, err := strconv.Atoi(f); err == nil {
			pids = append(pids, n)
		}
	}
	return pids
}

// bundleMarkers maps a recent-logcat signature to a bundle-source verdict. Order
// matters: a live-server / hot-reload marker (metro) is checked before the
// weaker "is this even React Native" markers.
var bundleMarkers = []struct {
	verdict string
	needle  string
}{
	{"metro", "HMRClient"},
	{"metro", "Fast Refresh"},
	{"metro", "DevServerHelper"},
	{"metro", "BundleDownloader"},
	{"metro", "Metro waiting"},
	{"metro", "Downloading JS bundle"},
	{"embedded", "Loading from assets"},
	{"embedded", "Unable to connect with runtime"},
	{"embedded", "loading embedded"},
}

// classifyBundle guesses whether the recent logcat came from a Metro-connected
// dev bundle or an embedded one, returning the verdict and the line it keyed on.
// Pure — unit-tested with synthetic logs (the heuristic can't be exercised
// without a running RN app). Verdicts: metro, embedded, not-react-native (no RN
// runtime at all), unknown (RN present but no bundle-source signal).
func classifyBundle(logs string) (verdict, evidence string) {
	for _, m := range bundleMarkers {
		if i := strings.Index(logs, m.needle); i >= 0 {
			return m.verdict, firstLineContaining(logs, m.needle)
		}
	}
	// No decisive marker. Is this even a React Native app?
	for _, rn := range []string{"ReactNativeJS", "ReactNative", "com.facebook.react", "expo"} {
		if strings.Contains(logs, rn) {
			return "unknown", ""
		}
	}
	return "not-react-native", ""
}

// firstLineContaining returns the first (trimmed) log line that contains needle.
func firstLineContaining(logs, needle string) string {
	for line := range strings.SplitSeq(logs, "\n") {
		if strings.Contains(line, needle) {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

// GetAppDetails reports an app's version and launchable activity via
// `dumpsys package` + `cmd package resolve-activity`.
func (c *Client) GetAppDetails(ctx context.Context, pkg string) (AppDetails, error) {
	d := AppDetails{Package: pkg}
	dump, err := c.adb(ctx, "shell", "dumpsys", "package", pkg)
	if err != nil {
		return d, err
	}
	if !strings.Contains(dump, "Unable to find package") && strings.Contains(dump, "Package [") {
		d.Installed = true
	}
	if m := versionNameRe.FindStringSubmatch(dump); m != nil {
		d.VersionName = m[1]
	}
	if m := versionCodeRe.FindStringSubmatch(dump); m != nil {
		d.VersionCode = m[1]
	}
	// Best-effort launcher activity.
	if res, err := c.adb(ctx, "shell", "cmd", "package", "resolve-activity",
		"--brief", "-c", "android.intent.category.LAUNCHER", pkg); err == nil {
		for line := range strings.SplitSeq(res, "\n") {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "/") && !strings.Contains(line, " ") {
				d.LauncherActivity = line
			}
		}
	}
	return d, nil
}
