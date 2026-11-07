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

// FileExists rejects directories on purpose, so using it to look for
// platform-tools/ reports every real SDK as missing. That shipped once: doctor
// declared a working SDK "not an SDK install" on the line above successfully
// running adb out of it.
func TestDirExistsAndIsSDKRoot(t *testing.T) {
	root := fakeSDK(t)
	tools := filepath.Join(root, "platform-tools")

	if !DirExists(tools) {
		t.Error("DirExists = false for a directory that exists")
	}
	if FileExists(tools) {
		t.Error("FileExists = true for a directory; IsSDKRoot must not be built on it")
	}
	if !IsSDKRoot(root) {
		t.Errorf("IsSDKRoot(%q) = false, want true", root)
	}

	// A path that exists but holds no platform-tools/ is not an SDK.
	if IsSDKRoot(t.TempDir()) {
		t.Error("IsSDKRoot = true for a directory with no platform-tools/")
	}
	if IsSDKRoot("") {
		t.Error("IsSDKRoot = true for an empty root")
	}
	// A *file* named platform-tools doesn't make an SDK either.
	odd := t.TempDir()
	if err := os.WriteFile(filepath.Join(odd, "platform-tools"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if IsSDKRoot(odd) {
		t.Error("IsSDKRoot = true where platform-tools is a file, not a directory")
	}
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
	t.Setenv("HOME", t.TempDir()) // no default install either, so nothing valid exists
	t.Setenv("ANDROID_SDK_ROOT", "")
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

func TestAapt2PathSortsBuildToolsNumerically(t *testing.T) {
	root := fakeSDK(t)
	mk := func(v string) string {
		dir := filepath.Join(root, "build-tools", v)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, Exe("aapt2"))
		if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	mk("9.0.0")
	mk("34.0.0")
	rc := mk("35.0.0-rc1")
	t.Setenv("ANDROID_HOME", root)
	t.Setenv("ANDROID_SDK_ROOT", "")
	if got := Aapt2Path(); got != rc {
		t.Errorf("9.0.0 must not outrank 34/35: got %s, want the 35.0.0-rc1 one %s", got, rc)
	}
	rel := mk("35.0.0")
	if got := Aapt2Path(); got != rel {
		t.Errorf("a release must outrank its rc: got %s, want %s", got, rel)
	}
}

func TestRootSkipsInvalidEnvForValidDefault(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Root()'s fallback reads USERPROFILE/AppData on Windows")
	}
	home := t.TempDir()
	fallback := filepath.Join(home, "Library", "Android", "sdk")
	if runtime.GOOS == "linux" {
		fallback = filepath.Join(home, "Android", "Sdk")
	}
	if err := os.MkdirAll(filepath.Join(fallback, "platform-tools"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("ANDROID_SDK_ROOT", "")
	t.Setenv("ANDROID_HOME", filepath.Join(t.TempDir(), "typo"))
	if got := Root(); got != fallback {
		t.Errorf("invalid ANDROID_HOME must fall through to the valid default: got %s, want %s", got, fallback)
	}
	good := fakeSDK(t)
	t.Setenv("ANDROID_HOME", good)
	if got := Root(); got != good {
		t.Errorf("a valid ANDROID_HOME still wins: got %s, want %s", got, good)
	}
	// Nothing valid anywhere: keep reporting the variable so doctor can name it.
	empty := t.TempDir()
	t.Setenv("HOME", empty)
	bad := filepath.Join(t.TempDir(), "typo")
	t.Setenv("ANDROID_HOME", bad)
	if got := Root(); got != bad {
		t.Errorf("with no valid SDK anywhere Root() should still report the set variable: got %s", got)
	}
}

func countKey(env []string, key string) int {
	n := 0
	for _, kv := range env {
		if k, _, _ := strings.Cut(kv, "="); strings.EqualFold(k, key) {
			n++
		}
	}
	return n
}

func TestAndroidHomeEnvSubstitutesInvalidCallerValue(t *testing.T) {
	good := fakeSDK(t)
	bad := filepath.Join(t.TempDir(), "typo")

	// A valid caller value is untouched.
	env := androidHomeEnv([]string{"ANDROID_HOME=" + good}, fakeSDK(t))
	if v, _ := envValue(env, "ANDROID_HOME"); v != good {
		t.Errorf("valid caller value changed to %q", v)
	}
	// Invalid ANDROID_HOME is replaced in place.
	env = androidHomeEnv([]string{"A=1", "ANDROID_HOME=" + bad}, good)
	if v, _ := envValue(env, "ANDROID_HOME"); v != good {
		t.Errorf("ANDROID_HOME = %q, want %q", v, good)
	}
	if n := countKey(env, "ANDROID_HOME"); n != 1 {
		t.Errorf("ANDROID_HOME appears %d times", n)
	}
	// Invalid ANDROID_SDK_ROOT cannot win.
	env = androidHomeEnv([]string{"ANDROID_SDK_ROOT=" + bad}, good)
	if v, _ := envValue(env, "ANDROID_HOME"); v != good {
		t.Errorf("ANDROID_HOME = %q, want %q", v, good)
	}
	if v, ok := envValue(env, "ANDROID_SDK_ROOT"); ok && v != "" && v != good {
		t.Errorf("invalid ANDROID_SDK_ROOT survived: %q", v)
	}
	// Both set and invalid.
	env = androidHomeEnv([]string{"ANDROID_HOME=" + bad, "ANDROID_SDK_ROOT=" + bad}, good)
	if v, _ := envValue(env, "ANDROID_HOME"); v != good {
		t.Errorf("ANDROID_HOME = %q, want %q", v, good)
	}
	if v, ok := envValue(env, "ANDROID_SDK_ROOT"); ok && v != "" && v != good {
		t.Errorf("invalid ANDROID_SDK_ROOT survived: %q", v)
	}
	// Valid ANDROID_HOME beside an invalid ANDROID_SDK_ROOT: valid one untouched.
	env = androidHomeEnv([]string{"ANDROID_HOME=" + good, "ANDROID_SDK_ROOT=" + bad}, fakeSDK(t))
	if v, _ := envValue(env, "ANDROID_HOME"); v != good {
		t.Errorf("valid ANDROID_HOME changed to %q", v)
	}
	// Nothing valid anywhere: unchanged.
	env = androidHomeEnv([]string{"ANDROID_HOME=" + bad}, bad)
	if v, _ := envValue(env, "ANDROID_HOME"); v != bad {
		t.Errorf("with no valid root the env must be unchanged, got %q", v)
	}
}
