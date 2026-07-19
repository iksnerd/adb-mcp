package adb

import "testing"

// The two JSON dumps are verbatim from a Pixel emulator: the first with no
// fingerprint enrolled, the second after enrolling exactly one.
const (
	fpDumpEmpty   = `Dumping for sensorId: 0, provider: FingerprintProvider` + "\n" + `{"service":"FingerprintProvider\/default","prints":[{"id":0,"count":0,"accept":0,"reject":0,"acquire":0,"lockout":0,"permanentLockout":0}]}`
	fpDumpOne     = `{"service":"FingerprintProvider\/default","prints":[{"id":0,"count":1,"accept":0,"reject":0}]}`
	fpDumpTwoUser = `{"prints":[{"id":0,"count":2,"accept":0},{"id":10,"count":1,"accept":0}]}`
	fpDumpLegacy  = "Fingerprint Manager state:\n  User 0: count: 3\n"
)

func TestParseEnrolledFingerprints(t *testing.T) {
	cases := []struct {
		name      string
		in        string
		wantCount int
		wantOK    bool
	}{
		{"empty", fpDumpEmpty, 0, true},
		{"one enrolled", fpDumpOne, 1, true},
		{"two users summed", fpDumpTwoUser, 3, true},
		{"legacy text", fpDumpLegacy, 3, true},
		{"unrecognised", "some other dump with no counts", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			count, ok := parseEnrolledFingerprints(tc.in)
			if count != tc.wantCount || ok != tc.wantOK {
				t.Errorf("parseEnrolledFingerprints = (%d,%v), want (%d,%v)", count, ok, tc.wantCount, tc.wantOK)
			}
		})
	}
}
