package guides

import (
	"strings"
	"testing"
)

func TestGuidesNeverUseAdbReverseShorthand(t *testing.T) {
	for _, g := range all {
		if strings.Contains(g.body, "adb_reverse tcp:") {
			t.Errorf("guide %s uses the adb_reverse tcp: shorthand; the tool takes {device_port: N}", g.name)
		}
	}
}

func TestBuildGuideCoversBackgroundBuilds(t *testing.T) {
	for _, g := range all {
		if g.name != "build-and-test" {
			continue
		}
		for _, want := range []string{"background", "build_status", "apk_path"} {
			if !strings.Contains(g.body, want) {
				t.Errorf("build-and-test guide never mentions %q", want)
			}
		}
		return
	}
	t.Fatal("build-and-test guide not registered")
}
