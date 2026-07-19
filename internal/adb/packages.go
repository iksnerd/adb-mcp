package adb

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"regexp"
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
