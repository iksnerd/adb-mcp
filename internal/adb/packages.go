package adb

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/iksnerd/adb-mcp/internal/concurrent"
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
var registeredSchemeRe = regexp.MustCompile(`(?mi)^\s*(?:scheme|schemes?)\s*[:=]\s*([a-z][a-z0-9+.-]*)\b`)

// RegisteredURLSchemes returns best-effort URL schemes declared by an
// installed package. Android's dumpsys layout varies by release, so callers
// should treat an empty result as "not discovered", not as proof that none
// are registered.
func (c *Client) RegisteredURLSchemes(ctx context.Context, pkg string) ([]string, error) {
	out, err := c.adb(ctx, "shell", "dumpsys", "package", pkg)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var schemes []string
	for _, m := range registeredSchemeRe.FindAllStringSubmatch(out, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			schemes = append(schemes, m[1])
		}
	}
	return schemes, nil
}

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

// isPackageInstalled reports whether a `dumpsys package <pkg>` dump describes
// an installed package. An unknown package prints "Unable to find package:
// <pkg>" instead of the "Package [<pkg>] (...):" header every installed
// package has.
func isPackageInstalled(dump string) bool {
	return !strings.Contains(dump, "Unable to find package") && strings.Contains(dump, "Package [")
}

// AppState is a runtime snapshot of an installed app: whether and where its
// process is running, and — for React Native / Expo dev builds — whether it is
// serving a live Metro bundle or its baked-in embedded one. Getting the last
// distinction wrong is the expensive failure: a dev client that silently fell
// back to its embedded bundle ignores every code edit, so taps and log reads run
// against stale code with no obvious signal. Two live processes for one package
// (a lingering old build + a fresh install) is the same class of trap — presses
// and log reads can hit different pids.
type AppState struct {
	Package        string       `json:"package"`
	Installed      bool         `json:"installed"`
	Running        bool         `json:"running"`
	Foreground     bool         `json:"foreground"`
	TopActivity    string       `json:"top_activity,omitempty"`
	PIDs           []int        `json:"pids,omitempty"`
	ProcessUptime  string       `json:"process_uptime,omitempty"` // wall-clock of the main process (ps ETIME)
	FirstInstall   string       `json:"first_install_time,omitempty"`
	LastUpdate     string       `json:"last_update_time,omitempty"`
	BundleSource   string       `json:"bundle_source"`             // metro | embedded | unknown | not-react-native
	BundleEvidence string       `json:"bundle_evidence,omitempty"` // the log line(s) the guess is based on
	BundleSignals  []string     `json:"bundle_signals,omitempty"`  // independent signals used by the verdict
	Metro          *MetroServer `json:"metro,omitempty"`           // WHICH dev server, not just "a" dev server
	SourceMTime    string       `json:"source_mtime,omitempty"`
	LastHMRUpdate  string       `json:"last_hmr_update,omitempty"`
	BundleStale    bool         `json:"bundle_stale,omitempty"`
	StaleVerdict   string       `json:"stale_verdict,omitempty"` // stale | current | undetermined (only when source_path was given)
	StaleReason    string       `json:"stale_reason,omitempty"`
	Notes          []string     `json:"notes,omitempty"`
}

// setStaleness records the staleness verdict on s. It is always called when the
// caller passed a source_path — including when the answer is "undetermined",
// which is the whole point: the field's previous behaviour was to compute
// nothing and say nothing when no HMR marker existed, so a caller reading a
// clean response concluded the JavaScript was current when it was several
// commits behind. Absence of a warning is not absence of a problem.
func (s *AppState) setStaleness(verdict, reason string) {
	s.StaleVerdict = verdict
	s.StaleReason = reason
	s.BundleStale = verdict == "stale"
}

// stalenessVerdict decides whether the JavaScript a process is executing still
// matches the host sources under sourcePath. Pure, so every branch — including
// the two that produce "stale" without any timestamp comparison — is unit
// tested. Order matters: a wrong-checkout Metro and an embedded bundle are
// decisive on their own, and neither leaves a timestamp to compare.
func stalenessVerdict(bundleSource, metroRoot, sourcePath string, sourceTime, markerTime time.Time, haveMarker bool) (verdict, reason string) {
	switch {
	case bundleSource == "embedded":
		return "stale", fmt.Sprintf("this process is running its EMBEDDED bundle, so nothing under %s is in it — adb_reverse tcp:8081, then relaunch", sourcePath)
	case metroRoot != "" && !SameTree(metroRoot, sourcePath):
		return "stale", fmt.Sprintf("the Metro dev server this process is connected to is rooted at %s, which is NOT %s — a dev server left running from another checkout still holds the port, so the running JavaScript is that checkout's code; kill it and start Metro from %s", metroRoot, sourcePath, sourcePath)
	case sourceTime.IsZero():
		return "undetermined", fmt.Sprintf("could not read a newest source mtime under %s", sourcePath)
	case bundleSource != "metro":
		return "undetermined", fmt.Sprintf("bundle_source is %q, so there is no confirmed Metro connection to compare source_mtime against", bundleSource)
	case !haveMarker:
		return "undetermined", "no epoch-timed Metro/HMR marker in this process's recent logcat, so source_mtime has nothing to compare against. A live Metro socket proves the process is CONNECTED, not that the bundle it is running came from this checkout — clear_logcat, reload_app, then call app_state again"
	case sourceTime.After(markerTime.Add(2 * time.Second)):
		return "stale", fmt.Sprintf("source files under %s are newer (%s) than the latest observed Metro/HMR update (%s) — Metro's watcher misses git-driven file replacement (checkout/stash), so reload_app before trusting anything on screen",
			sourcePath, sourceTime.Format(time.RFC3339), markerTime.Format(time.RFC3339))
	default:
		return "current", fmt.Sprintf("the latest Metro/HMR update (%s) is at or after the newest source file under %s (%s)",
			markerTime.Format(time.RFC3339), sourcePath, sourceTime.Format(time.RFC3339))
	}
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
	return c.GetAppStateWithSource(ctx, pkg, "")
}

// GetAppStateWithSource is GetAppState plus an optional host source path. When
// provided, it compares the newest source mtime with the latest epoch-timed
// Metro/HMR marker in logcat. This catches git checkout/stash replacements that
// Metro's watcher missed while the process still has a live Metro socket.
func (c *Client) GetAppStateWithSource(ctx context.Context, pkg, sourcePath string) (AppState, error) {
	s := AppState{Package: pkg, BundleSource: "unknown"}
	var sourceTime time.Time
	if sourcePath != "" {
		if t, err := newestSourceMTime(sourcePath); err == nil {
			sourceTime = t
			s.SourceMTime = t.Format(time.RFC3339)
		} else {
			s.Notes = append(s.Notes, fmt.Sprintf("source path unavailable: %v", err))
		}
	}

	if dump, err := c.adb(ctx, "shell", "dumpsys", "package", pkg); err == nil {
		s.Installed = isPackageInstalled(dump)
		if m := firstInstallRe.FindStringSubmatch(dump); m != nil {
			s.FirstInstall = strings.TrimSpace(m[1])
		}
		if m := lastUpdateRe.FindStringSubmatch(dump); m != nil {
			s.LastUpdate = strings.TrimSpace(m[1])
		}
	}
	if !s.Installed {
		s.Notes = append(s.Notes, "package not installed (or dumpsys package couldn't read it)")
		if sourcePath != "" {
			s.setStaleness("undetermined", "the package is not installed, so there is no running bundle to compare against")
		}
		return s, nil
	}

	if out, err := c.adb(ctx, "shell", "pidof", pkg); err == nil {
		s.PIDs = parsePIDs(out)
	}
	s.Running = len(s.PIDs) > 0
	if !s.Running {
		s.Notes = append(s.Notes, "not running — launch_app first; bundle source can't be determined for a stopped app")
		s.BundleSource = "n/a"
		if sourcePath != "" {
			s.setStaleness("undetermined", "the app is not running, so there is no bundle to compare against — launch_app, then call app_state again")
		}
		return s, nil
	}
	// Foreground activity, process uptime, and the logcat dump below only depend
	// on s.PIDs[0], which is already known — run all three adb round-trips
	// concurrently instead of one after another, then apply the same
	// post-processing as before in the original order.
	pid := strconv.Itoa(s.PIDs[0])
	var activity, psOut, logs string
	var activityErr, psErr, logsErr error
	concurrent.RunAll(
		func() { activity, activityErr = c.ResumedActivity(ctx) },
		func() { psOut, psErr = c.adb(ctx, "shell", "ps", "-o", "ETIME=", "-p", pid) },
		func() {
			logs, logsErr = c.adb(ctx, "shell", "logcat", "-d", "-v", "epoch", "-t", "4000", "--pid", pid)
		},
	)

	if activityErr == nil {
		s.TopActivity = activity
		s.Foreground = strings.HasPrefix(activity, pkg+"/")
	} else {
		s.Notes = append(s.Notes, fmt.Sprintf("foreground activity unavailable: %v", activityErr))
	}
	if len(s.PIDs) > 1 {
		s.Notes = append(s.Notes, fmt.Sprintf("%d live processes for this package — taps and log reads may be hitting different ones; stop_app then launch_app for a clean single process", len(s.PIDs)))
	}
	if psErr == nil {
		s.ProcessUptime = strings.TrimSpace(psOut)
	}

	// Bundle source: scan the app's recent logcat for dev-server / hot-reload
	// markers. --pid keeps it to this app's own lines. Modern Expo/RN builds can
	// connect to Metro without emitting the older HMRClient/Fast Refresh markers,
	// so use the process' live TCP sockets as a second, independent signal.
	var markerTime time.Time
	var haveMarker bool
	if logsErr == nil {
		s.BundleSource, s.BundleEvidence = classifyBundle(logs)
		if markerTime, haveMarker = bundleUpdateTime(logs); haveMarker {
			s.LastHMRUpdate = markerTime.Format(time.RFC3339)
		}
		if s.BundleSource == "metro" || s.BundleSource == "embedded" {
			s.BundleSignals = append(s.BundleSignals, "logcat")
		}
	} else {
		s.Notes = append(s.Notes, fmt.Sprintf("recent logcat unavailable (%v) — the bundle verdict rests on socket evidence alone", logsErr))
	}
	// The socket probe runs whenever the process could still be talking to a dev
	// server — not just when logcat came back inconclusive. It is the only thing
	// that reports WHICH port, and the port is what identifies the dev server;
	// "on Metro" without "which Metro" is the reassuring-but-wrong answer a stray
	// dev server from another checkout produces.
	if s.BundleSource == "metro" || s.BundleSource == "unknown" {
		if sockets, err := c.adb(ctx, "shell", "cat", "/proc/"+pid+"/net/tcp", "/proc/"+pid+"/net/tcp6"); err == nil {
			if port, ok := metroSocketPort(sockets); ok {
				if s.BundleSource != "metro" {
					s.BundleSource = "metro"
					s.BundleEvidence = fmt.Sprintf("process has an established TCP connection to Metro port %d", port)
				}
				s.BundleSignals = append(s.BundleSignals, "live_socket")
				srv, resolved := HostMetroServer(ctx, port)
				s.Metro = &srv
				if !resolved {
					s.Notes = append(s.Notes, fmt.Sprintf("couldn't identify the host process listening on %d (no lsof, or the server is on another machine) — 'metro' here means A dev server, not necessarily the one serving your checkout", port))
				}
			}
		}
	}
	switch s.BundleSource {
	case "metro":
		note := "serving a live Metro bundle — code edits apply after a reload_app / dev-menu Reload"
		if s.Metro != nil && s.Metro.ProjectRoot != "" {
			note += fmt.Sprintf("; that dev server (pid %d, %s) is rooted at %s — confirm this is the checkout you are reasoning about, because a stray Metro from another branch holding the same port looks identical here", s.Metro.PID, s.Metro.Command, s.Metro.ProjectRoot)
		}
		s.Notes = append(s.Notes, note)
	case "embedded":
		s.Notes = append(s.Notes, "running the EMBEDDED bundle — your JS edits are NOT in this process; if you expected Metro, run adb_reverse tcp:8081 and relaunch")
	case "not-react-native":
		s.BundleSource = "n/a"
	case "unknown":
		s.Notes = append(s.Notes, "couldn't tell Metro from embedded — no React-Native markers in recent logcat; clear_logcat, reload, and re-check, or it may be a native (non-RN) app")
	}
	if sourcePath != "" {
		metroRoot := ""
		if s.Metro != nil {
			metroRoot = s.Metro.ProjectRoot
		}
		verdict, reason := stalenessVerdict(s.BundleSource, metroRoot, sourcePath, sourceTime, markerTime, haveMarker)
		s.setStaleness(verdict, reason)
	}
	return s, nil
}

func newestSourceMTime(root string) (time.Time, error) {
	info, err := os.Stat(root)
	if err != nil {
		return time.Time{}, err
	}
	newest := info.ModTime()
	if !info.IsDir() {
		return newest, nil
	}
	err = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			base := info.Name()
			if path != root && (base == ".git" || base == "node_modules" || base == "build" || base == ".gradle") {
				return filepath.SkipDir
			}
			return nil
		}
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	return newest, err
}

// metroSocketPort reports the first established connection to a conventional
// Metro/Expo dev-server port. /proc/net/tcp represents ports as uppercase
// hexadecimal; state 01 is TCP_ESTABLISHED. This is deliberately a fallback:
// a socket proves a live dev-server connection, but not that every app uses
// React Native, so logcat remains the primary embedded/not-RN signal.
func metroSocketPort(procNet string) (int, bool) {
	for line := range strings.SplitSeq(procNet, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[0] == "sl" || fields[3] != "01" {
			continue
		}
		parts := strings.Split(fields[2], ":")
		if len(parts) != 2 {
			continue
		}
		port, err := strconv.ParseInt(parts[1], 16, 32)
		if err != nil {
			continue
		}
		switch int(port) {
		case 8081, 8082, 19000, 19001, 19002:
			return int(port), true
		}
	}
	return 0, false
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

// bundleUpdateTime returns the timestamp of the MOST RECENT Metro/HMR marker
// line in logs. `logcat -d` is chronological (oldest first), so this scans
// every match rather than returning on the first — an early marker from
// right after app start would otherwise shadow a live reload that happened
// seconds ago, understating freshness and producing a false bundle_stale.
func bundleUpdateTime(logs string) (time.Time, bool) {
	var latest time.Time
	found := false
	for line := range strings.SplitSeq(logs, "\n") {
		if !isBundleUpdateLine(line) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		seconds, err := strconv.ParseFloat(fields[0], 64)
		if err != nil {
			continue
		}
		sec := int64(seconds)
		nsec := int64((seconds - float64(sec)) * 1e9)
		t := time.Unix(sec, nsec)
		if !found || t.After(latest) {
			latest = t
			found = true
		}
	}
	return latest, found
}

func isBundleUpdateLine(line string) bool {
	for _, marker := range bundleMarkers {
		if marker.verdict == "metro" && strings.Contains(line, marker.needle) {
			return true
		}
	}
	return false
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
	d.Installed = isPackageInstalled(dump)
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
