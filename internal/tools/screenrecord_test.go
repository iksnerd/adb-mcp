package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/iksnerd/adb-mcp/internal/adb"
)

// A recording started on a display that is asleep comes out empty, so say so
// at the moment it can still be fixed.
func TestStartScreenRecordWarnsWhenDisplayAsleep(t *testing.T) {
	fakeAdb(t, `case "$*" in *"dumpsys power"*) echo "  mWakefulness=Dozing";; esac`)
	t.Cleanup(adb.StopAllCaptures)
	res, err := startScreenRecord(context.Background(), serialArg{})
	if err != nil {
		t.Fatal(err)
	}
	got := resultText(t, res)
	for _, want := range []string{"Recording", "asleep", "stay_awake"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q should mention %q", got, want)
		}
	}
}

func TestStartScreenRecordSilentWhenDisplayAwake(t *testing.T) {
	fakeAdb(t, `case "$*" in *"dumpsys power"*) echo "  mWakefulness=Awake";; esac`)
	t.Cleanup(adb.StopAllCaptures)
	res, err := startScreenRecord(context.Background(), serialArg{})
	if err != nil {
		t.Fatal(err)
	}
	if got := resultText(t, res); strings.Contains(got, "asleep") {
		t.Errorf("an awake display should not warn: %q", got)
	}
}
