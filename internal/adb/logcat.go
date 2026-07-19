package adb

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Logcat dumps recent log lines and applies LogFilter
// (substring/priority/tags), dropping chatty spam. The window is either the
// last `lines` lines (adb logcat -t <count>), or — when `since` is set (e.g.
// "2m", "90s") — everything from that long ago on the DEVICE clock
// (adb logcat -t '<time>'), which is the right axis when the question is
// "the user just hit an error": on a chatty emulator a few hundred lines can
// span less than ten seconds.
//
// The format is pinned to `threadtime` (rather than the device default) so
// LogFilter's per-line priority/tag parsing is deterministic across devices —
// logLineRe expects the "… PID TID PRIO TAG:" shape threadtime produces.
func (c *Client) Logcat(ctx context.Context, lines int, since string, f LogFilter) (string, error) {
	if err := f.validate(); err != nil {
		return "", err
	}
	window := ""
	if strings.TrimSpace(since) != "" {
		cutoff, err := c.deviceCutoff(ctx, since)
		if err != nil {
			return "", err
		}
		window = cutoff
	} else {
		if lines <= 0 {
			lines = 400
		}
		window = strconv.Itoa(lines)
	}
	out, err := c.adb(ctx, "logcat", "-d", "-v", "threadtime", "-t", window)
	if err != nil {
		return "", err
	}
	return f.apply(out), nil
}

// ClearLogcat empties the device's logcat ring buffer (adb logcat -c) so the
// next read contains only lines produced after this call — the sharpest way to
// isolate what a single action logged.
func (c *Client) ClearLogcat(ctx context.Context) error {
	_, err := c.adb(ctx, "logcat", "-c")
	return err
}

// deviceCutoff computes the device-local "MM-DD HH:MM:SS.mmm" timestamp that
// lies `since` ago, using the device's own clock and timezone (host and device
// clocks can disagree, and logcat timestamps are device-local).
func (c *Client) deviceCutoff(ctx context.Context, since string) (string, error) {
	// The format is single-quoted so the DEVICE shell keeps "%s %z" as one
	// token: `adb shell` concatenates its args and the device shell re-parses
	// them, so an unquoted "+%s %z" would split into `date +%s %z`, dropping the
	// %z operand (same footgun escapeInputText guards against). Without the
	// quotes the reply loses its timezone field and deviceCutoff fails, which
	// breaks every `logcat since=…` call.
	now, err := c.adb(ctx, "shell", "date", "'+%s %z'")
	if err != nil {
		return "", fmt.Errorf("read device clock: %w", err)
	}
	cutoff, err := sinceCutoff(strings.TrimSpace(now), since)
	if err != nil {
		return "", err
	}
	return cutoff, nil
}

// LogFilter narrows a raw logcat dump. All set fields must match (AND); Tags
// itself is an OR across its entries. Zero value keeps everything (minus
// chatty noise, which is always stripped).
type LogFilter struct {
	// Substring is a case-insensitive substring to keep anywhere in the line.
	Substring string
	// Priority is a minimum level to keep: V, D, I, W, E, or F — matching
	// adb's own "*:E"-style filterspec (E keeps Error and Fatal). Empty means
	// no priority filtering.
	Priority string
	// Tags keeps only lines whose log tag contains one of these
	// (case-insensitive, OR'd). Empty means no tag filtering.
	Tags []string
}

// sinceCutoff turns a device clock reading ("<epoch> <±HHMM>", from
// `date +'%s %z'`) and a lookback ("2m", "90s", "1h30m", or a bare number of
// seconds) into the device-local "MM-DD HH:MM:SS.mmm" timestamp logcat's
// -t '<time>' expects.
func sinceCutoff(deviceNow, since string) (string, error) {
	fields := strings.Fields(deviceNow)
	if len(fields) != 2 {
		return "", fmt.Errorf("unexpected device clock output %q (want \"<epoch> <±HHMM>\")", deviceNow)
	}
	epoch, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return "", fmt.Errorf("unexpected device epoch %q", fields[0])
	}
	offset, err := parseTZOffset(fields[1])
	if err != nil {
		return "", err
	}
	dur, err := parseLookback(since)
	if err != nil {
		return "", err
	}
	t := time.Unix(epoch, 0).In(time.FixedZone("device", offset)).Add(-dur)
	return t.Format("01-02 15:04:05.000"), nil
}

// parseTZOffset parses a strftime %z offset like "+0200" or "-0530" into
// seconds east of UTC.
func parseTZOffset(s string) (int, error) {
	if len(s) != 5 || (s[0] != '+' && s[0] != '-') {
		return 0, fmt.Errorf("unexpected device timezone offset %q (want ±HHMM)", s)
	}
	hh, err1 := strconv.Atoi(s[1:3])
	mm, err2 := strconv.Atoi(s[3:5])
	if err1 != nil || err2 != nil {
		return 0, fmt.Errorf("unexpected device timezone offset %q (want ±HHMM)", s)
	}
	off := hh*3600 + mm*60
	if s[0] == '-' {
		off = -off
	}
	return off, nil
}

// parseLookback accepts a Go-style duration ("2m", "90s", "1h30m") or a bare
// number of seconds, and requires it to be positive.
func parseLookback(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if secs, err := strconv.Atoi(s); err == nil {
		s = strconv.Itoa(secs) + "s"
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("since must be a duration like \"2m\", \"90s\", or \"1h30m\", got %q", s)
	}
	if dur <= 0 {
		return 0, fmt.Errorf("since must be positive, got %q", s)
	}
	return dur, nil
}

// priorityRank orders Android log priorities low to high, matching adb's
// filterspec semantics: a minimum priority keeps that level and everything
// more severe.
var priorityRank = map[string]int{"V": 0, "D": 1, "I": 2, "W": 3, "E": 4, "F": 5}

// logLineRe matches a `threadtime`-format logcat header:
// "MM-DD HH:MM:SS.mmm  PID  TID X TAG   : message" — capturing the priority
// letter and tag (padding around the tag is trimmed by the caller). Both
// Logcat and StartLogcatCapture pin -v threadtime so this parse is reliable.
var logLineRe = regexp.MustCompile(`^\S+\s+\S+\s+\d+\s+\d+\s+([VDIWEF])\s+([^:]*):`)

func (f LogFilter) validate() error {
	if f.Priority == "" {
		return nil
	}
	if _, ok := priorityRank[strings.ToUpper(strings.TrimSpace(f.Priority))]; !ok {
		return fmt.Errorf("priority must be one of V, D, I, W, E, F, got %q", f.Priority)
	}
	return nil
}

// apply filters raw multi-line logcat output, keeping lines that pass every
// set criterion. Lines whose header doesn't parse (rare: adb status lines, a
// wrapped continuation without its own header) are kept rather than dropped,
// since priority/tag filtering can't be evaluated for them.
func (f LogFilter) apply(raw string) string {
	needle := strings.ToLower(strings.TrimSpace(f.Substring))
	minRank, hasPriority := priorityRank[strings.ToUpper(strings.TrimSpace(f.Priority))]
	tags := make([]string, 0, len(f.Tags))
	for _, t := range f.Tags {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
			tags = append(tags, t)
		}
	}

	var kept []string
	for line := range strings.SplitSeq(raw, "\n") {
		if isChattyNoise(line) {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(line), needle) {
			continue
		}
		if (hasPriority || len(tags) > 0) && !matchesPriorityAndTags(line, minRank, hasPriority, tags) {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func matchesPriorityAndTags(line string, minRank int, hasPriority bool, tags []string) bool {
	prio, tag, ok := parseLogLine(line)
	if !ok {
		return true // can't evaluate; don't drop
	}
	if hasPriority {
		if rank, known := priorityRank[prio]; known && rank < minRank {
			return false
		}
	}
	if len(tags) > 0 {
		tagLower := strings.ToLower(tag)
		matched := false
		for _, t := range tags {
			if strings.Contains(tagLower, t) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// parseLogLine extracts the priority letter and tag from one `-v time` /
// default-format logcat line.
func parseLogLine(line string) (priority, tag string, ok bool) {
	m := logLineRe.FindStringSubmatch(line)
	if m == nil {
		return "", "", false
	}
	return m[1], strings.TrimSpace(m[2]), true
}

// isChattyNoise reports whether a logcat line is Android "chatty" dedup spam,
// emitted under the chatty tag as "uid=… expire N lines" / "… identical N
// lines". It requires both the chatty marker and the dedup phrasing so a real
// log line that merely mentions the word "chatty" (an app tag, package name, or
// message text) is not silently dropped.
func isChattyNoise(line string) bool {
	if !strings.Contains(line, "chatty") {
		return false
	}
	return strings.Contains(line, "expire ") || strings.Contains(line, "identical ")
}
