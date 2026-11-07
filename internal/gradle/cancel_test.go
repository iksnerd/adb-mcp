package gradle

import (
	"strings"
	"testing"
	"time"
)

func TestCancelledBuildSummarySaysSo(t *testing.T) {
	dir := fakeProject(t, `exec sleep 30`)
	job, err := StartBuild(dir, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := CancelBuild(job.ID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		s, _ := BuildStatus(job.ID)
		if s.Status == StatusFailed {
			if !strings.Contains(s.Err, "cancelled by cancel_build") {
				t.Fatalf("Err = %q, want it to say the build was cancelled", s.Err)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("cancelled build never reported failed")
}
