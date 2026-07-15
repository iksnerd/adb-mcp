package android

import (
	"strings"
	"testing"
)

const sampleDropbox = `Drop box contents: 3 entries
========================================
2026-07-15 14:53:11 data_app_crash (text, 40 bytes)
Process: com.example.old
java.lang.IllegalStateException: old one
========================================
2026-07-15 19:13:29 data_app_crash (text, 40 bytes)
Process: com.example.newest
java.lang.NullPointerException: newest crash
	at com.facebook.react.ReactActivity.onKeyDown()
========================================
2026-07-15 16:00:00 data_app_crash (text, 40 bytes)
Process: com.other.app
java.lang.RuntimeException: middle other`

func TestLatestDropboxEntry(t *testing.T) {
	// No package filter → most recent by timestamp (19:13:29, the newest NPE).
	entry, ts, ok := latestDropboxEntry(sampleDropbox, "")
	if !ok {
		t.Fatal("expected an entry")
	}
	if ts != "2026-07-15 19:13:29" {
		t.Errorf("ts = %q, want the newest 2026-07-15 19:13:29", ts)
	}
	if !strings.Contains(entry, "newest crash") {
		t.Errorf("entry = %q, want the newest crash", entry)
	}

	// Package filter → most recent entry mentioning that package, even though a
	// newer crash from another app exists.
	entry, ts, ok = latestDropboxEntry(sampleDropbox, "com.other.app")
	if !ok || ts != "2026-07-15 16:00:00" || !strings.Contains(entry, "middle other") {
		t.Errorf("filtered entry = %q ts=%q, want the com.other.app crash", entry, ts)
	}

	// A package with no crash → not found.
	if _, _, ok := latestDropboxEntry(sampleDropbox, "com.nope"); ok {
		t.Error("expected no entry for a package with no crash")
	}

	// Empty / preamble-only input → not found.
	if _, _, ok := latestDropboxEntry("Drop box contents: 0 entries", ""); ok {
		t.Error("expected no entry for empty dropbox")
	}
}

func TestTruncateCrash(t *testing.T) {
	if got := truncateCrash("short"); got != "short" {
		t.Errorf("truncateCrash kept-short = %q", got)
	}
	big := strings.Repeat("x", maxCrashChars+50)
	got := truncateCrash(big)
	if len(got) <= maxCrashChars || !strings.Contains(got, "truncated") {
		t.Errorf("truncateCrash did not truncate/annotate a large crash (len=%d)", len(got))
	}
}
