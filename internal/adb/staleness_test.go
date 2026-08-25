package adb

import (
	"strings"
	"testing"
	"time"
)

func TestStalenessVerdict(t *testing.T) {
	src := "/Users/dev/src/MyApp"
	now := time.Unix(1_700_000_000, 0)
	older := now.Add(-time.Hour)
	newer := now.Add(time.Hour)

	cases := []struct {
		name         string
		bundleSource string
		metroRoot    string
		sourceTime   time.Time
		markerTime   time.Time
		haveMarker   bool
		want         string
		reasonHas    string
	}{
		{
			name:         "embedded bundle is stale by definition",
			bundleSource: "embedded",
			sourceTime:   now, markerTime: now, haveMarker: true,
			want: "stale", reasonHas: "EMBEDDED",
		},
		{
			// The 2026-08-25 field case: bundle_source "metro", a live socket,
			// everything reading healthy — and the dev server belonged to a
			// different checkout, so the running JS was another branch's.
			name:         "metro rooted in another checkout is stale",
			bundleSource: "metro",
			metroRoot:    "/Users/dev/src/OtherBranch",
			sourceTime:   older, markerTime: now, haveMarker: true,
			want: "stale", reasonHas: "OtherBranch",
		},
		{
			name:         "metro rooted in this checkout is not a mismatch",
			bundleSource: "metro",
			metroRoot:    src,
			sourceTime:   older, markerTime: now, haveMarker: true,
			want: "current",
		},
		{
			name:         "unreadable source mtime is undetermined",
			bundleSource: "metro",
			sourceTime:   time.Time{}, markerTime: now, haveMarker: true,
			want: "undetermined", reasonHas: "newest source mtime",
		},
		{
			name:         "no confirmed metro connection is undetermined",
			bundleSource: "unknown",
			sourceTime:   now, markerTime: time.Time{}, haveMarker: false,
			want: "undetermined", reasonHas: "no confirmed Metro connection",
		},
		{
			// The reported failure: source_path was passed in exactly its
			// documented scenario and the response carried no verdict at all,
			// which read as "fine". It must say so out loud instead.
			name:         "metro with no HMR marker is undetermined, not silent",
			bundleSource: "metro",
			sourceTime:   now, markerTime: time.Time{}, haveMarker: false,
			want: "undetermined", reasonHas: "clear_logcat",
		},
		{
			name:         "source newer than the last HMR update is stale",
			bundleSource: "metro",
			sourceTime:   newer, markerTime: now, haveMarker: true,
			want: "stale", reasonHas: "newer",
		},
		{
			name:         "HMR update after the newest source file is current",
			bundleSource: "metro",
			sourceTime:   older, markerTime: now, haveMarker: true,
			want: "current",
		},
		{
			// Guard the 2s tolerance: an edit landing a moment after the marker
			// is clock skew, not a stale bundle.
			name:         "sub-tolerance skew is not stale",
			bundleSource: "metro",
			sourceTime:   now.Add(time.Second), markerTime: now, haveMarker: true,
			want: "current",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, reason := stalenessVerdict(tc.bundleSource, tc.metroRoot, src, tc.sourceTime, tc.markerTime, tc.haveMarker)
			if got != tc.want {
				t.Errorf("verdict = %q, want %q (reason: %s)", got, tc.want, reason)
			}
			if reason == "" {
				t.Error("every verdict must carry a reason — a bare verdict is the silence this replaced")
			}
			if tc.reasonHas != "" && !strings.Contains(reason, tc.reasonHas) {
				t.Errorf("reason %q does not mention %q", reason, tc.reasonHas)
			}
		})
	}
}

func TestSetStalenessTracksBundleStale(t *testing.T) {
	var s AppState
	s.setStaleness("stale", "because")
	if !s.BundleStale || s.StaleVerdict != "stale" || s.StaleReason != "because" {
		t.Errorf("stale: got %+v", s)
	}
	s.setStaleness("undetermined", "no marker")
	if s.BundleStale {
		t.Error("undetermined must not set bundle_stale — it is not a claim that the bundle is fresh OR stale")
	}
}
