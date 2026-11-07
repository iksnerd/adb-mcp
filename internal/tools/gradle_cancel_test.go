package tools

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/iksnerd/adb-mcp/internal/gradle"
)

func fakeGradleProject(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake gradlew is a shell script")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gradlew"), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func waitBuild(t *testing.T, id, want string) gradle.BuildSummary {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		s, err := gradle.BuildStatus(id)
		if err != nil {
			t.Fatal(err)
		}
		if s.Status == want {
			return s
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("job %s never reached %s", id, want)
	return gradle.BuildSummary{}
}

func TestCancelBuildStopsRunningJob(t *testing.T) {
	dir := fakeGradleProject(t, `exec sleep 30`)
	job, err := gradle.StartBuild(dir, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := cancelBuild(context.Background(), cancelBuildArgs{JobID: job.ID})
	if err != nil {
		t.Fatal(err)
	}
	got := resultText(t, res)
	if !strings.Contains(got, "Cancellation requested") || !strings.Contains(got, "build_status") || !strings.Contains(got, "failed") {
		t.Errorf("result = %q, want cancellation requested and a build_status/failed note", got)
	}
	s := waitBuild(t, job.ID, gradle.StatusFailed)
	if !strings.Contains(s.Err, "cancelled") {
		t.Errorf("Err = %q, want cancelled", s.Err)
	}
}

func TestCancelBuildFinishedJobSaysSo(t *testing.T) {
	dir := fakeGradleProject(t, `echo "BUILD SUCCESSFUL"`)
	job, err := gradle.StartBuild(dir, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	waitBuild(t, job.ID, gradle.StatusPassed)
	res, err := cancelBuild(context.Background(), cancelBuildArgs{JobID: job.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got := resultText(t, res); !strings.Contains(got, "already finished") {
		t.Errorf("result = %q, want it to say the job already finished", got)
	}
}

func TestCancelBuildUnknownJob(t *testing.T) {
	_, err := cancelBuild(context.Background(), cancelBuildArgs{JobID: "nope"})
	if err == nil || !strings.Contains(err.Error(), "unknown build job") {
		t.Errorf("err = %v, want unknown build job", err)
	}
}

func TestCancelBuildRejectsBlankJobID(t *testing.T) {
	_, err := cancelBuild(context.Background(), cancelBuildArgs{JobID: "  "})
	if err == nil || !strings.Contains(err.Error(), "job_id is required") {
		t.Errorf("err = %v, want job_id is required", err)
	}
}
