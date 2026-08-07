package adb

import (
	"reflect"
	"testing"
	"time"
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

func TestMetroSocketPort(t *testing.T) {
	procNet := `  sl  local_address rem_address   st
   3: 0100007F:9C40 0200007F:1F91 01 00000000:00000000 01:00000014 00000000 1000 0 12345 4 0000000000000000 20 4 31 10 -1`
	if got, ok := metroSocketPort(procNet); !ok || got != 8081 {
		t.Fatalf("metroSocketPort() = (%d, %t), want (8081, true)", got, ok)
	}
	if got, ok := metroSocketPort("1: 0100007F:9C40 0200007F:2328 06"); ok || got != 0 {
		t.Fatalf("metroSocketPort(non-established) = (%d, %t), want (0, false)", got, ok)
	}
}

func TestBundleUpdateTime(t *testing.T) {
	got, ok := bundleUpdateTime("1775550000.250  I  HMRClient: connection established")
	if !ok || !got.Equal(time.Unix(1775550000, 250000000)) {
		t.Fatalf("bundleUpdateTime() = %v, %v", got, ok)
	}
	if _, ok := bundleUpdateTime("I HMRClient: connection established"); ok {
		t.Fatal("expected non-epoch logcat timestamp to be ignored")
	}
}

// TestBundleUpdateTimeUsesLatestMarker guards against regressing to the first
// match: logcat -d is chronological (oldest first), so an early marker from
// right after app start must not shadow a live reload seconds ago.
func TestBundleUpdateTimeUsesLatestMarker(t *testing.T) {
	logs := "1775550000.000  I  HMRClient: connection established\n" +
		"1775550120.500  I  Fast Refresh: applied\n"
	got, ok := bundleUpdateTime(logs)
	if !ok || !got.Equal(time.Unix(1775550120, 500000000)) {
		t.Fatalf("bundleUpdateTime() = %v, %v, want the later (Fast Refresh) marker", got, ok)
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
