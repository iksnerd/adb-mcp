package gradle

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const junitPass = `<?xml version="1.0"?><testsuite name="S" tests="2" failures="0" errors="0" skipped="0" time="1"><testcase name="a" classname="S"/><testcase name="b" classname="S"/></testsuite>`

func setMtime(t *testing.T, path string, mod time.Time) {
	t.Helper()
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
}

func TestParseTestResultsSinceIgnoresStaleReports(t *testing.T) {
	dir := t.TempDir()
	start := time.Now()
	stale := filepath.Join(dir, "app", "build", "test-results", "testDebugUnitTest", "TEST-Old.xml")
	fresh := filepath.Join(dir, "app", "build", "test-results", "testReleaseUnitTest", "TEST-New.xml")
	for _, p := range []string{stale, fresh} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, p, junitPass)
	}
	setMtime(t, stale, start.Add(-time.Hour))
	setMtime(t, fresh, start.Add(time.Second))

	sum, found := ParseTestResultsSince(dir, start)
	if !found || sum.Tests != 2 || sum.Suites != 1 {
		t.Errorf("got found=%v tests=%d suites=%d, want only the fresh report (2 tests, 1 suite)", found, sum.Tests, sum.Suites)
	}

	setMtime(t, fresh, start.Add(-time.Hour))
	if _, found := ParseTestResultsSince(dir, start); found {
		t.Error("found = true with only stale reports, want false")
	}
	// A zero start time keeps the old "any report" behaviour.
	if _, found := ParseTestResultsSince(dir, time.Time{}); !found {
		t.Error("zero since should accept any report")
	}
}

func TestParseTestResultsSkipsVendorDirs(t *testing.T) {
	dir := t.TempDir()
	for _, rel := range []string{"node_modules/x/build/test-results/t/TEST-a.xml", ".gradle/y/test-results/t/TEST-b.xml", ".git/z/test-results/t/TEST-c.xml"} {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, p, junitPass)
	}
	if _, found := ParseTestResults(dir); found {
		t.Error("reports under node_modules/.gradle/.git must be ignored")
	}
}

func TestFindCoverageReportsSinceIgnoresStale(t *testing.T) {
	dir := t.TempDir()
	start := time.Now()
	old := filepath.Join(dir, "app", "build", "reports", "jacoco", "t", "t.xml")
	if err := os.MkdirAll(filepath.Dir(old), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, old, `<report/>`)
	setMtime(t, old, start.Add(-time.Hour))
	if _, err := FindCoverageReportsSince(dir, start); err == nil {
		t.Error("expected an error when the only report predates the run")
	}
	fresh := filepath.Join(dir, "core", "build", "reports", "jacoco", "t", "t.xml")
	if err := os.MkdirAll(filepath.Dir(fresh), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, fresh, `<report/>`)
	got, err := FindCoverageReportsSince(dir, start)
	if err != nil || len(got) != 1 || got[0] != fresh {
		t.Errorf("got %v, %v; want only %s", got, err, fresh)
	}
}

func TestFindCoverageReportsSkipsVendorDirs(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "node_modules", "m", "build", "reports", "jacoco", "t", "t.xml")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, p, `<report/>`)
	if _, err := FindCoverageReports(dir); err == nil {
		t.Error("report under node_modules must be ignored")
	}
}

func TestVariantFromTask(t *testing.T) {
	cases := map[string]string{
		"assembleDebug":            "debug",
		"assembleFooRelease":       "fooRelease",
		":app:assembleFreeDebug":   "freeDebug",
		"assemble":                 "",
		"assembleDebugAndroidTest": "",
		"assembleReleaseUnitTest":  "",
		"bundleRelease":            "",
		"installDebug":             "",
		"":                         "",
	}
	for task, want := range cases {
		if got := VariantFromTask(task); got != want {
			t.Errorf("VariantFromTask(%q) = %q, want %q", task, got, want)
		}
	}
}

func TestPickAPKForBuildPrefersRequestedVariant(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	start := now.Add(-time.Minute)
	// assembleDebug was UP-TO-DATE; a release APK from another build is newer.
	debug := writeAPK(t, root, "app/build/outputs/apk/debug/app-debug.apk", start.Add(-time.Hour))
	release := writeAPK(t, root, "app/build/outputs/apk/release/app-release.apk", now)
	apks := FindAPKs(root)
	if apks[0] != release {
		t.Fatalf("setup: newest should be release, got %v", apks)
	}
	got, note, err := PickAPKForBuild(apks, "assembleDebug", start)
	if err != nil || got != debug {
		t.Fatalf("got %q, %v; want %q", got, err, debug)
	}
	if !strings.Contains(note, "predates") {
		t.Errorf("note = %q, want a stale/up-to-date note", note)
	}
}

func TestPickAPKForBuildFlavorLayout(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	free := writeAPK(t, root, "app/build/outputs/apk/free/release/app-free-release.apk", now.Add(-time.Hour))
	writeAPK(t, root, "app/build/outputs/apk/paid/release/app-paid-release.apk", now)
	got, _, err := PickAPKForBuild(FindAPKs(root), "assembleFreeRelease", now.Add(-2*time.Hour))
	if err != nil || got != free {
		t.Errorf("got %q, %v; want %q", got, err, free)
	}
}

func TestPickAPKForBuildNoMatchIsError(t *testing.T) {
	root := t.TempDir()
	writeAPK(t, root, "app/build/outputs/apk/release/app-release.apk", time.Now())
	_, _, err := PickAPKForBuild(FindAPKs(root), "assembleDebug", time.Now().Add(-time.Minute))
	if err == nil || !strings.Contains(err.Error(), "debug") {
		t.Errorf("err = %v, want an error naming the debug variant", err)
	}
}

func TestPickAPKForBuildNoVariantFallsBack(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	apk := writeAPK(t, root, "app/build/outputs/apk/debug/app-debug.apk", now)
	got, note, err := PickAPKForBuild(FindAPKs(root), "build", now.Add(-time.Minute))
	if err != nil || got != apk || note != "" {
		t.Errorf("got %q %q %v", got, note, err)
	}
}

func TestGenerateWrapperPinsGradleVersion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script")
	}
	bin := t.TempDir()
	argsFile := filepath.Join(bin, "args.txt")
	script := "#!/bin/sh\necho \"$@\" > " + argsFile + "\n"
	if err := os.WriteFile(filepath.Join(bin, "gradle"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	_, ok, err := GenerateWrapper(context.Background(), t.TempDir())
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	b, _ := os.ReadFile(argsFile)
	want := "wrapper --gradle-version " + WrapperGradleVersion
	if strings.TrimSpace(string(b)) != want {
		t.Errorf("args = %q, want %q", b, want)
	}
}
