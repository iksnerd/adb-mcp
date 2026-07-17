// Package sdk resolves the local Android SDK: where adb and the emulator live,
// and the environment they need to run. It is the leaf both the adb device
// layer and the gradle build layer depend on, so neither has to re-derive SDK
// paths.
package sdk

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Root resolves the Android SDK location, checking the standard environment
// variables first and falling back to the per-platform default install path.
func Root() string {
	for _, env := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"} {
		if v := os.Getenv(env); v != "" {
			return v
		}
	}
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
// binaries regardless of the caller's environment.
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
	for i, kv := range env {
		if key, val, found := strings.Cut(kv, "="); found && strings.EqualFold(key, "PATH") {
			env[i] = key + "=" + extra + string(os.PathListSeparator) + val
			return env
		}
	}
	return append(env, "PATH="+extra)
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
