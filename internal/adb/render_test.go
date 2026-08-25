package adb

import "testing"

// A trimmed but structurally faithful `dumpsys gfxinfo <pkg>` dump.
const gfxDump = `Applications Graphics Acceleration Info:
Uptime: 913245 Realtime: 913245

** Graphics info for pid 4113 [com.example.app] **

Stats since: 84636070394ns
Total frames rendered: 1240
Janky frames: 56 (4.52%)
50th percentile: 6ms
90th percentile: 12ms
95th percentile: 18ms
99th percentile: 45ms
Number Missed Vsync: 3
Number High input latency: 0
Number Slow UI thread: 12
Number Slow bitmap uploads: 0

View hierarchy:

  com.example.app/com.example.MainActivity/android.view.ViewRootImpl@a1b2c3
  1,548 views, 3666.43 kB of render nodes
  com.example.app/PopupWindow:d4e5f6/android.view.ViewRootImpl@d4e5f6
  12 views, 24.10 kB of render nodes

Total ViewRootImpl: 2
Total Views: 1560
Total DisplayList: 3690.53 kB
`

func TestParseGfxInfo(t *testing.T) {
	s := parseGfxInfo(gfxDump)
	if s.PID != 4113 {
		t.Errorf("PID = %d, want 4113", s.PID)
	}
	if s.Views != 1560 {
		t.Errorf("Views = %d, want the dump's own total (1560)", s.Views)
	}
	if s.RenderNodeKB != 3690.53 {
		t.Errorf("RenderNodeKB = %v, want 3690.53", s.RenderNodeKB)
	}
	if len(s.Windows) != 2 {
		t.Fatalf("Windows = %d, want one per ViewRootImpl", len(s.Windows))
	}
	// The comma-grouped count is the shape a real device prints for a big list.
	if s.Windows[0].Views != 1548 || s.Windows[0].RenderNodeKB != 3666.43 {
		t.Errorf("first window = %+v, want 1548 views / 3666.43 kB", s.Windows[0])
	}
	if s.Windows[0].Name == "" {
		t.Error("window name should carry the preceding ViewRootImpl line")
	}
	if s.TotalFrames != 1240 || s.JankyFrames != 56 || s.JankyPercent != 4.52 {
		t.Errorf("frame stats = %d/%d (%v%%), want 56/1240 (4.52%%)", s.JankyFrames, s.TotalFrames, s.JankyPercent)
	}
	if s.P50MS != 6 || s.P90MS != 12 || s.P95MS != 18 || s.P99MS != 45 {
		t.Errorf("percentiles = %d/%d/%d/%d ms", s.P50MS, s.P90MS, s.P95MS, s.P99MS)
	}
	if s.MissedVsync != 3 || s.SlowUIThread != 12 {
		t.Errorf("MissedVsync=%d SlowUIThread=%d, want 3 and 12", s.MissedVsync, s.SlowUIThread)
	}
}

// Older releases print the per-window counts with no totals block; gfxinfo's
// layout varies by Android version, so the parser sums rather than reporting 0.
func TestParseGfxInfoWithoutTotalsBlock(t *testing.T) {
	dump := `** Graphics info for pid 900 [com.example.app] **

View hierarchy:

  com.example.app/android.view.ViewRootImpl@aaa
  100 views, 10.50 kB of render nodes
  com.example.app/android.view.ViewRootImpl@bbb
  40 views, 4.50 kB of render nodes
`
	s := parseGfxInfo(dump)
	if s.Views != 140 {
		t.Errorf("Views = %d, want the sum of the per-window lines (140)", s.Views)
	}
	if s.RenderNodeKB != 15.0 {
		t.Errorf("RenderNodeKB = %v, want 15.0", s.RenderNodeKB)
	}
}

func TestParseGfxInfoEmpty(t *testing.T) {
	s := parseGfxInfo("")
	if s.Views != 0 || s.TotalFrames != 0 || len(s.Windows) != 0 {
		t.Errorf("an empty dump must parse to zeroes, got %+v", s)
	}
}

func TestParseSkippedFrames(t *testing.T) {
	logs := `01-01 00:00:01.000  4113  4113 I Choreographer: Skipped 47 frames!  The application may be doing too much work on its main thread.
01-01 00:00:02.000  4113  4113 I Choreographer: Skipped 112 frames!  The application may be doing too much work on its main thread.
01-01 00:00:03.000  4113  4113 D SomethingElse: unrelated line
01-01 00:00:04.000  4113  4113 I Choreographer: Skipped 8 frames!
`
	events, worst := parseSkippedFrames(logs)
	if events != 3 {
		t.Errorf("events = %d, want 3", events)
	}
	if worst != 112 {
		t.Errorf("worst = %d, want the largest burst (112)", worst)
	}
	if e, w := parseSkippedFrames("nothing here"); e != 0 || w != 0 {
		t.Errorf("clean logs = (%d, %d), want (0, 0)", e, w)
	}
}
