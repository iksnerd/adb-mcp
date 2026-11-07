// Package sdk resolves the local Android SDK: where adb and the emulator live,
// and the environment they need to run. It is the leaf both the adb device
// layer and the gradle build layer depend on, so neither has to re-derive SDK
// paths.
package sdk

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

// Root resolves the Android SDK location, checking the standard environment
// variables first and falling back to the per-platform default install path.
//
// A variable that is set but does not point at an SDK (a typo, a stale path)
// does not win over a valid variable or a valid default install; otherwise it
// would hide behind a bare "adb not found". When nothing valid exists, the
// first set variable is still returned so doctor can name it.
func Root() string {
	var badEnv string
	for _, env := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"} {
		if v := os.Getenv(env); v != "" {
			if IsSDKRoot(v) {
				return v
			}
			if badEnv == "" {
				badEnv = v
			}
		}
	}
	def := defaultRoot()
	if badEnv != "" && !IsSDKRoot(def) {
		return badEnv
	}
	return def
}

func defaultRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Android", "sdk")
	case "windows":
		return filepath.Join(home, "AppData", "Local", "Android", "Sdk")
	default: // linux
		return filepath.Join(home, "Android", "Sdk")
	}
}

// AdbPath returns the path to the adb binary. If it cannot be found under the
// resolved SDK root, it returns the bare name so a PATH lookup can still work.
func AdbPath() string {
	if root := Root(); root != "" {
		p := filepath.Join(root, "platform-tools", Exe("adb"))
		if FileExists(p) {
			return p
		}
	}
	return Exe("adb")
}

// EmulatorPath returns the path to the emulator binary, mirroring AdbPath.
func EmulatorPath() string {
	if root := Root(); root != "" {
		p := filepath.Join(root, "emulator", Exe("emulator"))
		if FileExists(p) {
			return p
		}
	}
	return Exe("emulator")
}

// CommandEnv returns an environment with the SDK tool directories prepended to
// PATH, so adb/emulator (and gradle's toolchain) resolve their own helper
// binaries regardless of the caller's environment. It also exports ANDROID_HOME
// when the caller's environment lacks it — see androidHomeEnv.
func CommandEnv() []string {
	env := os.Environ()
	root := Root()
	if root == "" {
		return env
	}
	extra := filepath.Join(root, "platform-tools") + string(os.PathListSeparator) +
		filepath.Join(root, "emulator")
	// Match the PATH key case-insensitively: on Windows it is typically "Path",
	// and env keys are case-insensitive there — appending a second "PATH=" would
	// clobber the system path (Go dedupes env case-insensitively on Windows).
	// Preserve the original key and value so nothing is lost.
	pathSet := false
	for i, kv := range env {
		if key, val, found := strings.Cut(kv, "="); found && strings.EqualFold(key, "PATH") {
			env[i] = key + "=" + extra + string(os.PathListSeparator) + val
			pathSet = true
			break
		}
	}
	if !pathSet {
		env = append(env, "PATH="+extra)
	}
	return androidHomeEnv(env, root)
}

// androidHomeEnv exports ANDROID_HOME=root unless the caller already set
// ANDROID_HOME or ANDROID_SDK_ROOT.
//
// Root() falls back to the conventional per-platform install path when neither
// variable is set, which is enough for this server to find adb and the
// emulator itself — but the Android Gradle plugin does its own SDK lookup and
// knows nothing about that fallback. Without this, every Gradle tool failed
// with "SDK location not found" on any project that hadn't hand-written a
// local.properties, including one this server had just scaffolded. Only set
// when the directory really exists, so a wrong guess can't mask the clearer
// "no SDK" error with a confusing one about a bad path.
func androidHomeEnv(env []string, root string) []string {
	existing := -1
	for i, kv := range env {
		key, val, found := strings.Cut(kv, "=")
		if !found {
			continue
		}
		isHome := strings.EqualFold(key, "ANDROID_HOME")
		if val != "" && (isHome || strings.EqualFold(key, "ANDROID_SDK_ROOT")) {
			return env
		}
		if isHome {
			existing = i // set but empty — overwrite it rather than add a duplicate key
		}
	}
	if !IsSDKRoot(root) {
		return env
	}
	if existing >= 0 {
		env[existing] = "ANDROID_HOME=" + root
		return env
	}
	return append(env, "ANDROID_HOME="+root)
}

// Exe adds the platform executable suffix (.exe on Windows).
func Exe(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// FileExists reports whether p exists and is a regular file.
func FileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// DirExists is FileExists's counterpart for directories — FileExists rejects
// them on purpose, since its callers are looking for executables. Passing a
// directory to FileExists silently reports "missing", so anything checking for
// a directory (an SDK's platform-tools/, say) must use this instead.
func DirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// IsSDKRoot reports whether root looks like a real Android SDK install rather
// than a path that merely exists. Root() falls back to the conventional
// location whether or not anything is there, so callers that need to warn a
// user share this one predicate — doctor's message and the ANDROID_HOME export
// must agree about what counts as an SDK.
func IsSDKRoot(root string) bool {
	return root != "" && DirExists(filepath.Join(root, "platform-tools"))
}

// Aapt2Path returns the newest build-tools aapt2 under the SDK root, or the
// bare name so a PATH lookup can still work.
func Aapt2Path() string {
	if root := Root(); root != "" {
		versions, _ := filepath.Glob(filepath.Join(root, "build-tools", "*"))
		sort.Slice(versions, func(i, j int) bool {
			return versionLess(filepath.Base(versions[j]), filepath.Base(versions[i])) // newest first
		})
		for _, dir := range versions {
			if p := filepath.Join(dir, Exe("aapt2")); FileExists(p) {
				return p
			}
		}
	}
	return Exe("aapt2")
}

// versionLess orders build-tools directory names such as "34.0.0" or
// "35.0.0-rc1" by numeric components, so 9.0.0 < 34.0.0, and a release sorts
// above its own pre-release. Names that are not versions sort lowest.
func versionLess(a, b string) bool {
	an, apre, aok := splitVersion(a)
	bn, bpre, bok := splitVersion(b)
	if aok != bok {
		return !aok
	}
	if !aok {
		return a < b
	}
	for i := 0; i < len(an) || i < len(bn); i++ {
		var x, y int
		if i < len(an) {
			x = an[i]
		}
		if i < len(bn) {
			y = bn[i]
		}
		if x != y {
			return x < y
		}
	}
	switch {
	case apre == bpre:
		return false
	case apre == "":
		return false // a release is newer than any of its pre-releases
	case bpre == "":
		return true
	}
	return apre < bpre
}

func splitVersion(v string) (nums []int, pre string, ok bool) {
	core, pre, _ := strings.Cut(v, "-")
	for _, p := range strings.Split(core, ".") {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, "", false
		}
		nums = append(nums, n)
	}
	return nums, pre, true
}
