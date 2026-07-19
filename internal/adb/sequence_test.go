package adb

import (
	"context"
	"testing"
)

// hierWithCancel is a minimal uiautomator dump containing a "Cancel" element,
// enough to exercise the if_present/if_absent guards (the fake runner returns it
// for both the `uiautomator dump` and `cat` calls).
const hierWithCancel = `<?xml version='1.0' encoding='UTF-8'?><hierarchy rotation="0"><node text="Cancel" resource-id="" class="android.widget.Button" bounds="[0,0][100,100]" clickable="true"/></hierarchy>`

func TestRunSequenceGuards(t *testing.T) {
	ctx := context.Background()
	c, _ := newFake(hierWithCancel)

	res, err := c.RunSequence(ctx, []Step{
		{Action: "tap", X: 1, Y: 1, IfPresent: "Cancel"}, // guard satisfied → runs
		{Action: "tap", X: 2, Y: 2, IfPresent: "Nope"},   // not on screen → skipped
		{Action: "tap", X: 3, Y: 3, IfAbsent: "Cancel"},  // present, so absent-guard skips
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Aborted {
		t.Error("sequence should not have aborted")
	}
	want := []string{"ok", "skipped", "skipped"}
	if len(res.Steps) != len(want) {
		t.Fatalf("got %d step results, want %d", len(res.Steps), len(want))
	}
	for i, w := range want {
		if res.Steps[i].Status != w {
			t.Errorf("step %d status = %q, want %q", i, res.Steps[i].Status, w)
		}
	}
}

func TestRunSequenceAbortAndOptional(t *testing.T) {
	ctx := context.Background()
	c, _ := newFake(hierWithCancel)

	// A non-optional unknown action aborts the rest.
	res, _ := c.RunSequence(ctx, []Step{{Action: "bogus"}, {Action: "tap", X: 1, Y: 1}}, false)
	if !res.Aborted || len(res.Steps) != 1 || res.Steps[0].Status != "error" {
		t.Errorf("expected abort after step 0 error, got aborted=%v steps=%+v", res.Aborted, res.Steps)
	}

	// optional=true records the error but keeps going.
	res, _ = c.RunSequence(ctx, []Step{{Action: "bogus", Optional: true}, {Action: "tap", X: 1, Y: 1}}, false)
	if res.Aborted || len(res.Steps) != 2 {
		t.Fatalf("expected both steps to run, got aborted=%v steps=%d", res.Aborted, len(res.Steps))
	}
	if res.Steps[0].Status != "error" || res.Steps[1].Status != "ok" {
		t.Errorf("statuses = %q,%q; want error,ok", res.Steps[0].Status, res.Steps[1].Status)
	}
}

func TestRunSequenceSleepValidation(t *testing.T) {
	c, _ := newFake("")
	res, _ := c.RunSequence(context.Background(), []Step{{Action: "sleep", Seconds: 0}}, false)
	if res.Steps[0].Status != "error" {
		t.Errorf("sleep with no seconds should error, got %q", res.Steps[0].Status)
	}
}
