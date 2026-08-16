package sdk

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func envValue(env []string, key string) (string, bool) {
	for _, kv := range env {
		if k, v, found := strings.Cut(kv, "="); found && strings.EqualFold(k, key) {
			return v, true
		}
	}
	return "", false
}

// fakeSDK creates a directory that looks enough like an SDK install to pass
// the platform-tools existence check.
func fakeSDK(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "platform-tools"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

// The Android Gradle plugin does its own SDK lookup and knows nothing about
// Root()'s per-platform fallback, so the resolved root has to be exported or
// every Gradle tool fails with "SDK location not found".
//
// Both SDK variables are cleared so Root() must use its home-directory
// fallback: that is the case the export exists for, and asserting it with
// ANDROID_HOME already set would only read back what the test itself put there.
func TestCommandEnvExportsResolvedRootWhenUnset(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Root()'s fallback reads USERPROFILE/AppData on Windows")
	}
	home := t.TempDir()
	// Mirror the darwin/linux layout Root() falls back to.
	fallback := filepath.Join(home, "Library", "Android", "sdk")
	if runtime.GOOS == "linux" {
		fallback = filepath.Join(home, "Android", "Sdk")
	}
	if err := os.MkdirAll(filepath.Join(fallback, "platform-tools"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("ANDROID_HOME", "")
	t.Setenv("ANDROID_SDK_ROOT", "")

	got, ok := envValue(CommandEnv(), "ANDROID_HOME")
	if !ok || got == "" {
		t.Fatal("ANDROID_HOME not exported; Gradle would report 'SDK location not found'")
	}
	if got != fallback {
		t.Errorf("ANDROID_HOME = %q, want the resolved root %q", got, fallback)
	}
}

// An explicit ANDROID_SDK_ROOT is the caller's choice; don't add a second,
// conflicting variable behind their back.
func TestCommandEnvKeepsExistingSdkRoot(t *testing.T) {
	t.Setenv("ANDROID_HOME", "")
	t.Setenv("ANDROID_SDK_ROOT", fakeSDK(t))

	if v, ok := envValue(CommandEnv(), "ANDROID_HOME"); ok && v != "" {
		t.Errorf("ANDROID_HOME = %q, want it left unset when ANDROID_SDK_ROOT is set", v)
	}
}

// Root() returns a plausible-looking default path even with no SDK installed.
// Exporting that would trade a clear "no SDK" error for a confusing one about
// a path that isn't there.
func TestCommandEnvSkipsNonexistentRoot(t *testing.T) {
	t.Setenv("ANDROID_HOME", filepath.Join(t.TempDir(), "not-an-sdk"))

	env := CommandEnv()
	if v, ok := envValue(env, "ANDROID_HOME"); ok && v != "" && !strings.Contains(v, "not-an-sdk") {
		t.Errorf("ANDROID_HOME = %q, unexpected value", v)
	}
	// PATH must still be extended regardless of whether the root looks real.
	if _, ok := envValue(env, "PATH"); !ok {
		t.Error("PATH missing from CommandEnv()")
	}
}

func TestCommandEnvPrependsToolDirsToPath(t *testing.T) {
	root := fakeSDK(t)
	t.Setenv("ANDROID_HOME", root)

	path, ok := envValue(CommandEnv(), "PATH")
	if !ok {
		t.Fatal("PATH missing from CommandEnv()")
	}
	if !strings.HasPrefix(path, filepath.Join(root, "platform-tools")) {
		t.Errorf("PATH = %q, want it to start with the SDK platform-tools dir", path)
	}
	// Exactly one PATH entry: a duplicate would clobber the system path.
	count := 0
	for _, kv := range CommandEnv() {
		if k, _, _ := strings.Cut(kv, "="); strings.EqualFold(k, "PATH") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("PATH appears %d times in CommandEnv(), want 1", count)
	}
}
