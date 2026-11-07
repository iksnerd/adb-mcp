package adb

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestLastCrashErrorsWhenEveryTagFails(t *testing.T) {
	c := &Client{Serial: "emulator-5554", run: func(context.Context, ...string) ([]byte, error) {
		return nil, errors.New("device offline")
	}}
	if _, found, err := c.LastCrash(context.Background(), ""); err == nil || found {
		t.Fatalf("found=%v err=%v, want an error when no tag could be read", found, err)
	}
}

func TestLastCrashToleratesOneFailingTag(t *testing.T) {
	c := &Client{Serial: "emulator-5554", run: func(_ context.Context, args ...string) ([]byte, error) {
		if args[len(args)-1] == "data_app_native_crash" {
			return nil, errors.New("no such tag")
		}
		return []byte(sampleDropbox), nil
	}}
	crash, found, err := c.LastCrash(context.Background(), "")
	if err != nil || !found || !strings.Contains(crash, "newest crash") {
		t.Fatalf("crash=%q found=%v err=%v, want the newest crash from the readable tag", crash, found, err)
	}
}

func TestLatestDropboxEntryBoundsPackageMatch(t *testing.T) {
	dump := `========================================
2026-07-15 10:00:00 data_app_crash (text, 40 bytes)
Process: com.example.debug
Package: com.example.debug v1
java.lang.RuntimeException: debug build
========================================
2026-07-15 09:00:00 data_app_crash (text, 40 bytes)
Process: com.example
Package: com.example v1
java.lang.RuntimeException: release build
========================================
2026-07-15 11:00:00 data_app_crash (text, 40 bytes)
Process: com.third
java.lang.RuntimeException: caused by com.example.ui.Foo calling in`
	entry, _, ok := latestDropboxEntry(dump, "com.example")
	if !ok || !strings.Contains(entry, "release build") {
		t.Fatalf("entry = %q ok=%v, want only the exact com.example process", entry, ok)
	}
	if _, _, ok := latestDropboxEntry(dump, "com.exam"); ok {
		t.Error("a package prefix must not match")
	}
}
