package adb

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// crashTags are the DropBox tags LastCrash scans, newest wins across them:
// data_app_crash covers JVM/React-Native app crashes, data_app_native_crash
// covers native (NDK) crashes.
var crashTags = []string{"data_app_crash", "data_app_native_crash"}

// maxCrashChars caps a returned crash so an unusually large native tombstone
// can't blow the token budget; a JVM/RN stack is normally well under this.
const maxCrashChars = 16000

// dropboxSeparator is the 40-'=' line dumpsys prints before each entry.
const dropboxSeparator = "========================================"

// crashHeaderRe matches an entry header like
// "2026-07-15 19:13:29 data_app_crash (text, 4101 bytes)", capturing the
// leading "YYYY-MM-DD HH:MM:SS" timestamp (lexically sortable == chronological).
var crashHeaderRe = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})\b`)

// LastCrash returns the most recent app crash from DropBox (via `dumpsys
// dropbox --print`), optionally filtered to entries mentioning pkg. found is
// false when no matching crash is recorded (not an error — a clean device
// simply has none).
func (c *Client) LastCrash(ctx context.Context, pkg string) (crash string, found bool, err error) {
	var best, bestTS string
	for _, tag := range crashTags {
		out, e := c.adb(ctx, "shell", "dumpsys", "dropbox", "--print", tag)
		if e != nil {
			continue // tag may be absent on this device; try the next
		}
		if entry, ts, ok := latestDropboxEntry(out, pkg); ok && ts >= bestTS {
			bestTS, best = ts, entry
		}
	}
	if best == "" {
		return "", false, nil
	}
	return truncateCrash(best), true, nil
}

// latestDropboxEntry parses `dumpsys dropbox --print` output and returns the
// most recent entry (by header timestamp) that mentions pkg (or the most recent
// overall when pkg is empty), along with its timestamp.
func latestDropboxEntry(out, pkg string) (entry, ts string, ok bool) {
	// Parts split on the separator: [preamble, entry, entry, ...]. The preamble
	// ("Drop box contents: N entries") has no header timestamp, so it's skipped
	// naturally by the header check below.
	for _, part := range strings.Split(out, dropboxSeparator) {
		part = strings.Trim(part, "\n")
		if part == "" {
			continue
		}
		m := crashHeaderRe.FindStringSubmatch(firstLine(part))
		if m == nil {
			continue
		}
		if pkg != "" && !strings.Contains(part, pkg) {
			continue
		}
		if m[1] >= ts { // lexical compare works for this fixed timestamp format
			ts, entry, ok = m[1], part, true
		}
	}
	return entry, ts, ok
}

// firstLine returns everything before the first newline in s.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func truncateCrash(s string) string {
	if len(s) <= maxCrashChars {
		return s
	}
	return fmt.Sprintf("%s\n… (%d more chars — truncated)", s[:maxCrashChars], len(s)-maxCrashChars)
}
