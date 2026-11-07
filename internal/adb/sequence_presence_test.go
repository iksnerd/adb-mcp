package adb

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func decodeSteps(t *testing.T, js string) []Step {
	t.Helper()
	var steps []Step
	if err := json.Unmarshal([]byte(js), &steps); err != nil {
		t.Fatal(err)
	}
	return steps
}

func hasInputCall(f *fakeRun, sub string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if strings.Contains(strings.Join(c, " "), sub) {
			return true
		}
	}
	return false
}

func TestSequenceTapWithoutCoordinatesIsRejected(t *testing.T) {
	c, f := newFake("")
	res, _ := c.RunSequence(context.Background(), decodeSteps(t, `[{"action":"tap"}]`), false)
	if len(res.Steps) != 1 || res.Steps[0].Status != "error" || !strings.Contains(res.Steps[0].Error, "'x'") {
		t.Fatalf("want an error naming the missing coordinate, got %+v", res.Steps)
	}
	if hasInputCall(f, "input tap") {
		t.Error("a tap with no coordinates must not reach the device")
	}
}

func TestSequenceSwipeNeedsAllFourCoordinates(t *testing.T) {
	c, f := newFake("")
	res, _ := c.RunSequence(context.Background(),
		decodeSteps(t, `[{"action":"swipe","x":1,"y":2,"x2":3}]`), false)
	if res.Steps[0].Status != "error" || !strings.Contains(res.Steps[0].Error, "'y2'") {
		t.Fatalf("want an error naming y2, got %+v", res.Steps)
	}
	if hasInputCall(f, "input swipe") {
		t.Error("an incomplete swipe must not reach the device")
	}
}

func TestSequenceExplicitZeroCoordinatesStillTap(t *testing.T) {
	c, f := newFake("")
	res, _ := c.RunSequence(context.Background(), decodeSteps(t, `[{"action":"tap","x":0,"y":0}]`), false)
	if res.Steps[0].Status != "ok" || !hasInputCall(f, "input tap 0 0") {
		t.Fatalf("an explicit (0,0) is a valid tap, got %+v", res.Steps)
	}
}

// A node that only carries a resource id and shares its parent's rect is hidden
// by FilterAuto; a guard naming it must still see it.
const hierWithWrapper = `<?xml version='1.0' encoding='UTF-8'?><hierarchy rotation="0"><node text="" resource-id="" class="android.view.ViewGroup" bounds="[0,0][100,100]" clickable="false"><node text="" resource-id="com.x:id/biometric_wrapper" class="android.view.ViewGroup" bounds="[0,0][100,100]" clickable="false"><node text="hi" resource-id="" class="android.widget.TextView" bounds="[0,0][50,50]" clickable="false"/><node text="yo" resource-id="" class="android.widget.TextView" bounds="[50,50][100,100]" clickable="false"/></node></node></hierarchy>`

func TestSequenceGuardSeesResourceIDWrapper(t *testing.T) {
	c, _ := newFake(hierWithWrapper)
	res, err := c.RunSequence(context.Background(), []Step{
		{Action: "tap", X: 1, Y: 1, IfPresent: "biometric_wrapper"},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Steps[0].Status != "ok" {
		t.Errorf("if_present naming a resource-id wrapper must see it, got %q", res.Steps[0].Status)
	}
}
