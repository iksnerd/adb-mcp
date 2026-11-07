package tools

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// paramDescription returns the schema description of one tool parameter, as an
// MCP client sees it.
func paramDescription(t *testing.T, tool, param string) string {
	t.Helper()
	for _, tl := range listedTools(t) {
		if tl.Name != tool {
			continue
		}
		schema, _ := tl.InputSchema.(map[string]any)
		props, _ := schema["properties"].(map[string]any)
		p, _ := props[param].(map[string]any)
		d, _ := p["description"].(string)
		if d == "" {
			t.Fatalf("%s.%s has no description", tool, param)
		}
		return d
	}
	t.Fatalf("tool %s not registered", tool)
	return ""
}

var defaultNumRe = regexp.MustCompile(`(?i)default (\d+)`)

// describedDefault parses "Default N" out of a description.
func describedDefault(t *testing.T, tool, param string) int {
	t.Helper()
	d := paramDescription(t, tool, param)
	m := defaultNumRe.FindStringSubmatch(d)
	if m == nil {
		t.Fatalf("%s.%s description states no numeric default: %q", tool, param, d)
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// describedBoolDefault parses "Default true/false" out of a description.
func describedBoolDefault(t *testing.T, tool, param string) bool {
	t.Helper()
	d := strings.ToLower(paramDescription(t, tool, param))
	switch {
	case strings.Contains(d, "default true"):
		return true
	case strings.Contains(d, "default false"):
		return false
	}
	t.Fatalf("%s.%s description states no boolean default: %q", tool, param, d)
	return false
}

// The default a handler applies must be the one its schema description tells
// the model about; both used to be spelled out separately.
func TestDefaultsMatchSchemaDescriptions(t *testing.T) {
	for _, tc := range []struct{ tool, param string }{
		{"tap_on_text", "partial"}, {"tap_element", "partial"}, {"wait_for_text", "partial"},
	} {
		if got, want := partialOrDefault(nil), describedBoolDefault(t, tc.tool, tc.param); got != want {
			t.Errorf("%s.%s: partialOrDefault(nil) = %v, description says %v", tc.tool, tc.param, got, want)
		}
	}
	f := false
	if partialOrDefault(&f) {
		t.Error("partialOrDefault(&false) = true, an explicit value must win")
	}
	if got, want := defaultCaptureFinal, describedBoolDefault(t, "run_sequence", "capture_final"); got != want {
		t.Errorf("defaultCaptureFinal = %v, run_sequence.capture_final description says %v", got, want)
	}
	if got, want := defaultStopLogcatTail, describedDefault(t, "stop_logcat_capture", "tail"); got != want {
		t.Errorf("defaultStopLogcatTail = %d, stop_logcat_capture.tail description says %d", got, want)
	}
	if got, want := tailOrDefault(0), describedDefault(t, "stop_logcat_capture", "tail"); got != want {
		t.Errorf("tailOrDefault(0) = %d, description says %d", got, want)
	}
	if got, want := screenshotMaxDim(nil), describedDefault(t, "screenshot", "max_dim"); got != want {
		t.Errorf("screenshotMaxDim(nil) = %d, description says %d", got, want)
	}
	zero, neg := 0, -1
	if screenshotMaxDim(&zero) != 0 || screenshotMaxDim(&neg) != -1 {
		t.Error("an explicit max_dim <= 0 must pass through (it disables downscaling)")
	}
}

// logcat's line default lives in the adb layer; the handler must not shadow it
// and the description must keep saying what the layer does.
func TestLogcatDefaultLinesMatchesDescription(t *testing.T) {
	calls := fakeAdb(t, "")
	if _, err := logcat(context.Background(), logcatArgs{}); err != nil {
		t.Fatal(err)
	}
	want := strconv.Itoa(describedDefault(t, "logcat", "lines"))
	var found bool
	for _, c := range calls() {
		if strings.Contains(c, "logcat -d") {
			found = true
			if !strings.HasSuffix(c, " -t "+want) {
				t.Errorf("logcat call %q does not end in -t %s (the described default)", c, want)
			}
		}
	}
	if !found {
		t.Errorf("no logcat -d call recorded: %v", calls())
	}
}
