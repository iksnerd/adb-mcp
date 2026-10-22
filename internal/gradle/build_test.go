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

// fakeProject writes a gradlew shell script with the given body into a temp
// project dir, so build behavior is tested through the public API without a
// real Gradle.
func fakeProject(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake gradlew is a shell script")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "gradlew"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRunBuildSuccessSummary(t *testing.T) {
	dir := fakeProject(t, `mkdir -p app/build/outputs/apk/debug && : > app/build/outputs/apk/debug/app-debug.apk
echo "> Task :app:assembleDebug"
echo "BUILD SUCCESSFUL in 1s"`)
	s := RunBuild(context.Background(), dir, "", nil)
	if s.Status != StatusPassed {
		t.Fatalf("Status = %q, want passed", s.Status)
	}
	if s.Task != "assembleDebug" {
		t.Errorf("Task = %q, want assembleDebug", s.Task)
	}
	if len(s.APKs) != 1 || !strings.HasSuffix(s.APKs[0], "app-debug.apk") {
		t.Errorf("APKs = %v", s.APKs)
	}
	if s.ErrorCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", s.ErrorCount)
	}
	log, err := os.ReadFile(s.LogPath)
	if err != nil || !strings.Contains(string(log), "BUILD SUCCESSFUL") {
		t.Errorf("log at %q missing full output: %v %q", s.LogPath, err, log)
	}
}

func TestRunBuildFailureCountsErrorsAndRedactsLog(t *testing.T) {
	dir := fakeProject(t, `echo "systemProp.repoPassword=hunter2"
echo "e: /src/A.kt:3:5 Unresolved reference: foo"
echo "e: /src/B.kt:9:1 Type mismatch"
echo "BUILD FAILED in 2s"
exit 1`)
	s := RunBuild(context.Background(), dir, "assembleRelease", nil)
	if s.Status != StatusFailed {
		t.Fatalf("Status = %q, want failed", s.Status)
	}
	if s.ErrorCount != 2 || len(s.FirstErrors) != 2 {
		t.Errorf("ErrorCount = %d, FirstErrors = %v, want 2 and 2", s.ErrorCount, s.FirstErrors)
	}
	log, _ := os.ReadFile(s.LogPath)
	if strings.Contains(string(log), "hunter2") {
		t.Errorf("log file leaked a secret: %q", log)
	}
	if s.Duration <= 0 {
		t.Errorf("Duration = %v, want > 0", s.Duration)
	}
}

func TestBackgroundBuildReturnsAtOnceThenReportsResult(t *testing.T) {
	dir := fakeProject(t, `sleep 1
echo "BUILD SUCCESSFUL in 1s"`)
	start := time.Now()
	job, err := StartBuild(dir, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatalf("StartBuild blocked for %v", time.Since(start))
	}
	if job.ID == "" || job.LogPath == "" {
		t.Fatalf("job = %+v, want id and log path", job)
	}
	running, err := BuildStatus(job.ID)
	if err != nil || running.Status != StatusRunning {
		t.Fatalf("immediate status = %+v, %v; want running", running, err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		s, _ := BuildStatus(job.ID)
		if s.Status == StatusPassed {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("background build never reported passed")
}

func TestBuildStatusUnknownJob(t *testing.T) {
	if _, err := BuildStatus("nope"); err == nil {
		t.Error("expected an error for an unknown job id")
	}
}

func TestFailedSummaryNamesErrorsAndLogPath(t *testing.T) {
	s := BuildSummary{
		Task: "assembleDebug", Status: StatusFailed, Duration: 3 * time.Second,
		ErrorCount: 2, FirstErrors: []string{"e: A.kt:3 Unresolved reference: foo", "e: B.kt:9 Type mismatch"},
		LogPath: "/tmp/adb-mcp-gradle/build-1.log", Err: "gradle assembleDebug failed: exit status 1",
	}
	got := s.String()
	for _, want := range []string{"Build failed (assembleDebug)", "Errors: 2", "Unresolved reference: foo", "Full log: /tmp/adb-mcp-gradle/build-1.log", "exit status 1"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary missing %q:\n%s", want, got)
		}
	}
}

func TestRunBuildSurfacesGradlesOwnFailureReason(t *testing.T) {
	dir := fakeProject(t, `echo "FAILURE: Build failed with an exception."
echo
echo "* What went wrong:"
echo "Task 'nosuchTask' not found in root project 'bridge'."
echo
echo "* Try:"
echo "> Run gradlew tasks to get a list of available tasks."
echo "BUILD FAILED in 1s"
exit 1`)
	s := RunBuild(context.Background(), dir, "nosuchTask", nil)
	if s.Status != StatusFailed {
		t.Fatalf("Status = %q, want failed", s.Status)
	}
	if s.ErrorCount != 1 || len(s.FirstErrors) != 1 || !strings.Contains(s.FirstErrors[0], "Task 'nosuchTask' not found") {
		t.Errorf("ErrorCount = %d, FirstErrors = %v; want Gradle's own reason as the one error", s.ErrorCount, s.FirstErrors)
	}
}
