package adb

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// RenderStats is the whole-tree counterpart to describe_ui. describe_ui reads
// the accessibility hierarchy, which Android scopes to the VIEWPORT: a long
// ScrollView's off-screen children are simply not in it, at any filter. That
// makes it the right instrument for "is X on screen?" and the wrong one for
// "how many views did this list actually mount?" — the question you have to
// answer to argue that a list is unvirtualised. gfxinfo counts the real view
// tree, and the Choreographer tally says what that cost in dropped frames.
type RenderStats struct {
	Package      string         `json:"package"`
	PID          int            `json:"pid,omitempty"`
	Views        int            `json:"views"`
	RenderNodeKB float64        `json:"render_node_kb"`
	Windows      []RenderWindow `json:"windows,omitempty"`

	TotalFrames  int     `json:"total_frames,omitempty"`
	JankyFrames  int     `json:"janky_frames,omitempty"`
	JankyPercent float64 `json:"janky_percent,omitempty"`
	P50MS        int     `json:"p50_ms,omitempty"`
	P90MS        int     `json:"p90_ms,omitempty"`
	P95MS        int     `json:"p95_ms,omitempty"`
	P99MS        int     `json:"p99_ms,omitempty"`
	MissedVsync  int     `json:"missed_vsync,omitempty"`
	SlowUIThread int     `json:"slow_ui_thread,omitempty"`

	SkippedFrameEvents int      `json:"skipped_frame_events"`
	MaxSkippedFrames   int      `json:"max_skipped_frames"`
	Notes              []string `json:"notes,omitempty"`
}

// RenderWindow is one ViewRootImpl's share of the view tree — an app with a
// dialog or a dev-client overlay up has more than one.
type RenderWindow struct {
	Name         string  `json:"name,omitempty"`
	Views        int     `json:"views"`
	RenderNodeKB float64 `json:"render_node_kb"`
}

var (
	gfxPIDRe          = regexp.MustCompile(`Graphics info for pid (\d+)`)
	gfxViewsRe        = regexp.MustCompile(`^\s*([\d,]+) views, ([\d.]+) kB of render nodes`)
	gfxTotalViewsRe   = regexp.MustCompile(`Total Views:\s*([\d,]+)`)
	gfxDisplayListRe  = regexp.MustCompile(`Total DisplayList:\s*([\d.]+) kB`)
	gfxTotalFramesRe  = regexp.MustCompile(`Total frames rendered:\s*(\d+)`)
	gfxJankyRe        = regexp.MustCompile(`Janky frames:\s*(\d+)\s*\(([\d.]+)%\)`)
	gfxPercentileRe   = regexp.MustCompile(`(\d+)th percentile:\s*(\d+)ms`)
	gfxMissedVsyncRe  = regexp.MustCompile(`Number Missed Vsync:\s*(\d+)`)
	gfxSlowUIThreadRe = regexp.MustCompile(`Number Slow UI thread:\s*(\d+)`)
	skippedFramesRe   = regexp.MustCompile(`Skipped (\d+) frames`)
)

// parseGfxInfo reads `dumpsys gfxinfo <pkg>`. Pure, so the field-report shapes
// (a totals block, per-ViewRootImpl lines only, or a dump with no frame stats
// at all because the app hasn't drawn yet) are all unit-tested. Every field is
// optional: gfxinfo's layout differs across Android releases and a missing
// section must degrade to zero rather than fail the call.
func parseGfxInfo(out string) RenderStats {
	var s RenderStats
	// A multi-process app prints one "Graphics info for pid" section per
	// process. Counts from different processes must not be mixed, so keep only
	// the first section (the app's main process).
	if locs := gfxPIDRe.FindAllStringIndex(out, 2); len(locs) == 2 {
		out = out[:locs[1][0]]
	}
	if m := gfxPIDRe.FindStringSubmatch(out); m != nil {
		s.PID, _ = strconv.Atoi(m[1])
	}
	// The per-window lines come in pairs: a ViewRootImpl name line followed by
	// its "N views, M kB of render nodes" counts.
	var prev string
	for line := range strings.SplitSeq(out, "\n") {
		if m := gfxViewsRe.FindStringSubmatch(line); m != nil {
			w := RenderWindow{Name: strings.TrimSpace(prev)}
			w.Views = atoiCommas(m[1])
			w.RenderNodeKB, _ = strconv.ParseFloat(m[2], 64)
			s.Windows = append(s.Windows, w)
		}
		if t := strings.TrimSpace(line); t != "" {
			prev = t
		}
	}
	// Prefer the dump's own totals; fall back to summing the per-window lines
	// (older releases print the counts without a totals block).
	if m := gfxTotalViewsRe.FindStringSubmatch(out); m != nil {
		s.Views = atoiCommas(m[1])
	} else {
		for _, w := range s.Windows {
			s.Views += w.Views
		}
	}
	if m := gfxDisplayListRe.FindStringSubmatch(out); m != nil {
		s.RenderNodeKB, _ = strconv.ParseFloat(m[1], 64)
	} else {
		for _, w := range s.Windows {
			s.RenderNodeKB += w.RenderNodeKB
		}
	}
	if m := gfxTotalFramesRe.FindStringSubmatch(out); m != nil {
		s.TotalFrames, _ = strconv.Atoi(m[1])
	}
	if m := gfxJankyRe.FindStringSubmatch(out); m != nil {
		s.JankyFrames, _ = strconv.Atoi(m[1])
		s.JankyPercent, _ = strconv.ParseFloat(m[2], 64)
	}
	for _, m := range gfxPercentileRe.FindAllStringSubmatch(out, -1) {
		ms, _ := strconv.Atoi(m[2])
		switch m[1] {
		case "50":
			s.P50MS = ms
		case "90":
			s.P90MS = ms
		case "95":
			s.P95MS = ms
		case "99":
			s.P99MS = ms
		}
	}
	if m := gfxMissedVsyncRe.FindStringSubmatch(out); m != nil {
		s.MissedVsync, _ = strconv.Atoi(m[1])
	}
	if m := gfxSlowUIThreadRe.FindStringSubmatch(out); m != nil {
		s.SlowUIThread, _ = strconv.Atoi(m[1])
	}
	return s
}

// atoiCommas parses a count that large-number dumps may group ("1,548").
func atoiCommas(s string) int {
	n, _ := strconv.Atoi(strings.ReplaceAll(strings.TrimSpace(s), ",", ""))
	return n
}

// parseSkippedFrames tallies Choreographer's "Skipped N frames!" warnings —
// the main-thread-blocked signal that turns a large view count from a number
// into a user-visible problem. Returns how many times it fired and the worst
// single burst.
func parseSkippedFrames(logs string) (events, worst int) {
	for _, m := range skippedFramesRe.FindAllStringSubmatch(logs, -1) {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		events++
		if n > worst {
			worst = n
		}
	}
	return events, worst
}

var logTimeRe = regexp.MustCompile(`^(\d\d-\d\d \d\d:\d\d:\d\d\.\d+)`)

// parseSkippedFramesSince is parseSkippedFrames limited to events logged after
// the threadtime timestamp `after` ("" counts everything). It also returns the
// newest timestamp seen on any matching line, which a reset stores as the next
// baseline. Lines without a leading timestamp can't be placed, so they count
// only when there is no baseline.
func parseSkippedFramesSince(logs, after string) (events, worst int, newest string) {
	for line := range strings.SplitSeq(logs, "\n") {
		m := skippedFramesRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		ts := ""
		if t := logTimeRe.FindStringSubmatch(line); t != nil {
			ts = t[1]
		}
		if ts > newest {
			newest = ts
		}
		if after != "" && ts <= after {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		events++
		if n > worst {
			worst = n
		}
	}
	return events, worst, newest
}

// skippedBaseline remembers, per serial+package, the newest Choreographer event
// seen when counters were last reset, so later scans count only newer events.
var (
	skippedBaselineMu sync.Mutex
	skippedBaseline   = map[string]string{}
)

// GetRenderStats reports the app's real view-tree size and recent frame health.
// reset zeroes gfxinfo's frame counters AFTER reading, so the next call measures
// only what happened in between — the way to attribute jank to one interaction
// instead of to the whole session.
func (c *Client) GetRenderStats(ctx context.Context, pkg string, reset bool) (RenderStats, error) {
	out, err := c.adb(ctx, "shell", "dumpsys", "gfxinfo", pkg)
	if err != nil {
		return RenderStats{}, err
	}
	s := parseGfxInfo(out)
	s.Package = pkg
	if s.Views == 0 && s.TotalFrames == 0 {
		s.Notes = append(s.Notes, "gfxinfo reported no view tree and no frame stats — the app is probably not running (app_state confirms), or it has not drawn yet")
	}
	// Choreographer logs to the app's own pid, so scope the scan the same way
	// app_state does rather than reading the whole device buffer.
	baseKey := c.Serial + "/" + pkg
	var pendingBaseline string
	if s.PID > 0 {
		if logs, err := c.adb(ctx, "shell", "logcat", "-d", "-t", "4000", "--pid", strconv.Itoa(s.PID)); err == nil {
			skippedBaselineMu.Lock()
			after := skippedBaseline[baseKey]
			skippedBaselineMu.Unlock()
			var newest string
			s.SkippedFrameEvents, s.MaxSkippedFrames, newest = parseSkippedFramesSince(logs, after)
			if reset && newest > after {
				// Applied below only if the gfxinfo reset itself succeeded.
				pendingBaseline = newest
			}
		} else {
			s.Notes = append(s.Notes, fmt.Sprintf("skipped-frame tally unavailable: %v", err))
		}
	}
	if reset {
		if _, err := c.adb(ctx, "shell", "dumpsys", "gfxinfo", pkg, "reset"); err != nil {
			s.Notes = append(s.Notes, fmt.Sprintf("counters were NOT reset: %v", err))
		} else {
			if pendingBaseline != "" {
				skippedBaselineMu.Lock()
				skippedBaseline[baseKey] = pendingBaseline
				skippedBaselineMu.Unlock()
			}
			s.Notes = append(s.Notes, "frame counters reset — the next render_stats call measures only what happens from now on")
		}
	}
	return s, nil
}
