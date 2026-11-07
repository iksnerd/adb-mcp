package adb

import (
	"context"
	"errors"
	"testing"
)

func TestResumedActivityErrorsWhenNothingParses(t *testing.T) {
	c, _ := newFake("mResumedActivity: none\nsomething else")
	if got, err := c.ResumedActivity(context.Background()); err == nil {
		t.Fatalf("ResumedActivity = %q, nil; want an error when neither regex matches", got)
	}
	if got, err := c.SettledResumedActivity(context.Background()); err == nil {
		t.Fatalf("SettledResumedActivity = %q, nil; want an error, not an empty == empty settle", got)
	}
}

func TestResumedActivityStillParses(t *testing.T) {
	c, _ := newFake("topResumedActivity=ActivityRecord{abc u0 com.example/.MainActivity t12}")
	got, err := c.ResumedActivity(context.Background())
	if err != nil || got != "com.example/.MainActivity" {
		t.Fatalf("ResumedActivity = %q, %v", got, err)
	}
}

func TestIsDeviceSecureErrorsWhenVerifyItselfFails(t *testing.T) {
	// verify fails for a reason that says nothing about the credential
	// (offline device); get-disabled would answer "false" and read as secure.
	c := &Client{Serial: "emulator-5554", run: func(_ context.Context, args ...string) ([]byte, error) {
		if len(args) >= 3 && args[2] == "verify" {
			return nil, errors.New("adb: device offline")
		}
		return []byte("false"), nil
	}}
	if secure, err := c.IsDeviceSecure(context.Background()); err == nil {
		t.Fatalf("secure=%v err=nil, want an error when verify itself failed", secure)
	}
}
