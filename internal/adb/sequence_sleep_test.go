package adb

import (
	"context"
	"testing"
	"time"
)

func TestSleepStepIsCappedAt300Seconds(t *testing.T) {
	if got := sleepDuration(1e9); got != 300*time.Second {
		t.Errorf("sleepDuration(1e9) = %v, want 300s", got)
	}
	if got := sleepDuration(0.5); got != 500*time.Millisecond {
		t.Errorf("sleepDuration(0.5) = %v", got)
	}
	// A cancelled context returns at once, so the huge value never really sleeps.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c, _ := newFake("")
	if _, _, err := c.runStep(ctx, &Step{Action: "sleep", Seconds: 1e9}); err == nil {
		t.Error("cancelled context should stop the sleep")
	}
}

func TestSleepDetailSaysWhenCapped(t *testing.T) {
	if got := sleepDetail(1000); got != "slept 300s (capped from 1000)" {
		t.Errorf("detail = %q", got)
	}
	if got := sleepDetail(2); got != "slept 2s" {
		t.Errorf("detail = %q", got)
	}
}

func TestSleepStepStillRejectsNonPositive(t *testing.T) {
	c, _ := newFake("")
	if _, _, err := c.runStep(context.Background(), &Step{Action: "sleep", Seconds: -1}); err == nil {
		t.Error("negative seconds must error")
	}
}
