package adb

import (
	"reflect"
	"testing"
)

func TestParsePIDs(t *testing.T) {
	cases := []struct {
		in   string
		want []int
	}{
		{"1419", []int{1419}},
		{"1419 2050 3\n", []int{1419, 2050, 3}},
		{"", nil},
		{"  \n", nil},
		{"notapid 42", []int{42}},
	}
	for _, tc := range cases {
		if got := parsePIDs(tc.in); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("parsePIDs(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestClassifyBundle(t *testing.T) {
	cases := []struct {
		name        string
		logs        string
		wantVerdict string
		wantEvid    bool // whether an evidence line is expected
	}{
		{"metro via HMRClient", "07-19 ... I HMRClient: connection established\n", "metro", true},
		{"metro via Fast Refresh", "some noise\nD ReactNativeJS: Running \"main\"\nFast Refresh enabled\n", "metro", true},
		{"embedded", "I ReactNativeJS: bridge\nLoading from assets://index.android.bundle\n", "embedded", true},
		{"rn but no bundle signal", "I ReactNativeJS: hello\nD ReactNative: init\n", "unknown", false},
		{"not react native at all", "I ActivityManager: Start proc\nD SettingsProvider: x\n", "not-react-native", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			verdict, evid := classifyBundle(tc.logs)
			if verdict != tc.wantVerdict {
				t.Errorf("verdict = %q, want %q", verdict, tc.wantVerdict)
			}
			if (evid != "") != tc.wantEvid {
				t.Errorf("evidence = %q, wantPresent=%v", evid, tc.wantEvid)
			}
		})
	}
}

// TestParseInstallTimes guards the dumpsys package time extraction (real lines
// from a Pixel emulator).
func TestParseInstallTimes(t *testing.T) {
	dump := `  Package [com.android.settings] (abc):
    firstInstallTime=2026-07-19 17:18:18
    lastUpdateTime=2026-07-19 17:20:00`
	if m := firstInstallRe.FindStringSubmatch(dump); m == nil || m[1] != "2026-07-19 17:18:18" {
		t.Errorf("firstInstall = %v", m)
	}
	if m := lastUpdateRe.FindStringSubmatch(dump); m == nil || m[1] != "2026-07-19 17:20:00" {
		t.Errorf("lastUpdate = %v", m)
	}
}
