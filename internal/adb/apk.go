package adb

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/iksnerd/adb-mcp/internal/sdk"
)

// APKInfo is what an APK file declares about itself, read without a device.
type APKInfo struct {
	Package          string `json:"package"`
	VersionName      string `json:"version_name,omitempty"`
	VersionCode      string `json:"version_code,omitempty"`
	LauncherActivity string `json:"launcher_activity,omitempty"`
}

var (
	badgingPackageRe  = regexp.MustCompile(`^package: name='([^']+)'`)
	badgingVersionRe  = regexp.MustCompile(`versionName='([^']*)'`)
	badgingCodeRe     = regexp.MustCompile(`versionCode='(\d+)'`)
	badgingLauncherRe = regexp.MustCompile(`^launchable-activity: name='([^']+)'`)
)

// ParseBadging reads `aapt2 dump badging` output. LauncherActivity is in
// `package/activity` form, the same shape get_app_details reports for an
// installed app. Empty output is an error: it is what aapt2 gives when it
// cannot read the file.
func ParseBadging(out string) (APKInfo, error) {
	var info APKInfo
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		if m := badgingPackageRe.FindStringSubmatch(line); m != nil {
			info.Package = m[1]
			if v := badgingVersionRe.FindStringSubmatch(line); v != nil {
				info.VersionName = v[1]
			}
			if v := badgingCodeRe.FindStringSubmatch(line); v != nil {
				info.VersionCode = v[1]
			}
		} else if m := badgingLauncherRe.FindStringSubmatch(line); m != nil && info.LauncherActivity == "" {
			info.LauncherActivity = m[1]
		}
	}
	if info.Package == "" {
		return APKInfo{}, fmt.Errorf("no package line in aapt2 badging output")
	}
	if info.LauncherActivity != "" {
		info.LauncherActivity = info.Package + "/" + info.LauncherActivity
	}
	return info, nil
}

// ReadAPK reports the package id, version and launcher activity of a local APK
// using the SDK's aapt2, with no device attached.
func ReadAPK(ctx context.Context, apkPath string) (APKInfo, error) {
	if _, err := os.Stat(apkPath); err != nil {
		return APKInfo{}, fmt.Errorf("apk not found: %s", apkPath)
	}
	aapt2 := sdk.Aapt2Path()
	cmd := exec.CommandContext(ctx, aapt2, "dump", "badging", apkPath)
	cmd.Env = sdk.CommandEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return APKInfo{}, fmt.Errorf("%s dump badging %s: %v\n%s", aapt2, apkPath, err, strings.TrimSpace(string(out)))
	}
	info, err := ParseBadging(string(out))
	if err != nil {
		return APKInfo{}, fmt.Errorf("%v (aapt2: %s) — is a build-tools version installed under the SDK?", err, aapt2)
	}
	return info, nil
}
