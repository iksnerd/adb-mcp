package gradle

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// writeAPK creates an empty file (and its parents) with the given mtime.
func writeAPK(t *testing.T, root string, rel string, mod time.Time) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFindAPKsNewestFirst(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	// A stale androidTest APK whose path sorts lexically BEFORE the fresh
	// debug APK — the exact case that used to make build_and_run install the
	// wrong artifact.
	stale := writeAPK(t, root, "app/build/outputs/apk/androidTest/debug/app-debug-androidTest.apk", now.Add(-time.Hour))
	fresh := writeAPK(t, root, "app/build/outputs/apk/debug/app-debug.apk", now)
	// Not under build/outputs — must be ignored.
	writeAPK(t, root, "app/libs/vendor.apk", now)

	apks := FindAPKs(root)
	if len(apks) != 2 {
		t.Fatalf("expected 2 APKs, got %v", apks)
	}
	if apks[0] != fresh || apks[1] != stale {
		t.Errorf("expected newest first [%s %s], got %v", fresh, stale, apks)
	}
}

func TestFindAPKsPrunesVendorDirs(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	// APK-shaped files inside pruned trees must not be found (and the dirs
	// must not even be walked).
	writeAPK(t, root, "node_modules/some-pkg/build/outputs/fake.apk", now)
	writeAPK(t, root, ".gradle/cache/build/outputs/fake.apk", now)
	real := writeAPK(t, root, "app/build/outputs/apk/debug/app-debug.apk", now)

	apks := FindAPKs(root)
	if len(apks) != 1 || apks[0] != real {
		t.Errorf("expected only %s, got %v", real, apks)
	}
}

func TestPickAPK(t *testing.T) {
	if got := PickAPK(nil); got != "" {
		t.Errorf("empty input: got %q", got)
	}
	// Newest-first list where the newest is an androidTest APK (e.g. right
	// after connectedAndroidTest): the app APK must still win.
	apks := []string{
		"app/build/outputs/apk/androidTest/debug/app-debug-androidTest.apk",
		"app/build/outputs/apk/debug/app-debug.apk",
	}
	if got := PickAPK(apks); got != apks[1] {
		t.Errorf("expected the non-test APK, got %q", got)
	}
	// Only test APKs: fall back to the first rather than failing.
	onlyTest := apks[:1]
	if got := PickAPK(onlyTest); got != onlyTest[0] {
		t.Errorf("expected fallback to first, got %q", got)
	}
}

// `gradle wrapper` can echo ~/.gradle/gradle.properties under --info-style
// init scripts, so GenerateWrapper's output is redacted like every other
// Gradle run, on both the success and the failure path.
func TestGenerateWrapperRedactsOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a POSIX shell stub for the system gradle")
	}
	for _, tc := range []struct {
		name string
		exit int
		ok   bool
	}{{"success", 0, true}, {"failure", 1, false}} {
		t.Run(tc.name, func(t *testing.T) {
			bin := t.TempDir()
			stub := "#!/bin/sh\necho 'systemProp.repoPassword=fake-wrapper-secret-555'\nexit " + strconv.Itoa(tc.exit) + "\n"
			if err := os.WriteFile(filepath.Join(bin, "gradle"), []byte(stub), 0o755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

			out, ok, err := GenerateWrapper(context.Background(), t.TempDir())
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v (err=%v)", ok, tc.ok, err)
			}
			if strings.Contains(out, "fake-wrapper-secret-555") {
				t.Errorf("credential reached GenerateWrapper output: %q", out)
			}
			if !strings.Contains(out, redactedMark) {
				t.Errorf("expected %q in output, got %q", redactedMark, out)
			}
		})
	}
}
