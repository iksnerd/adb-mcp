package adb

import (
	"context"
	"strings"
	"testing"
)

func TestEmuRejectsControlCharactersInEveryArgument(t *testing.T) {
	ctx := context.Background()
	lvl := 3
	cases := map[string]func(*Client) error{
		"sms text":   func(c *Client) error { return c.SendSMS(ctx, "555", "hi\nkill") },
		"sms number": func(c *Client) error { return c.SendSMS(ctx, "555\r\nkill", "hi") },
		"snapshot": func(c *Client) error {
			_, err := c.Snapshot(ctx, "save", "a\nkill")
			return err
		},
		"sensor name": func(c *Client) error { return c.SetSensor(ctx, "light\nkill", []float64{1}) },
		"speed":       func(c *Client) error { return c.Cellular(ctx, "", "", nil, "lte\nkill", "") },
		"delay":       func(c *Client) error { return c.Cellular(ctx, "", "", &lvl, "", "gprs\rkill") },
		"gsm number":  func(c *Client) error { return c.GSMCall(ctx, "call", "555\nkill") },
	}
	for name, call := range cases {
		c, f := newFake("OK")
		err := call(c)
		if err == nil || !strings.Contains(err.Error(), "control character") {
			t.Errorf("%s: want control character error, got %v", name, err)
		}
		for _, argv := range f.calls {
			for _, a := range argv {
				if strings.ContainsAny(a, "\r\n") {
					t.Errorf("%s: control char reached adb: %q", name, argv)
				}
			}
		}
	}
}

func TestEmuKOMatchesOnlyAtLineStart(t *testing.T) {
	ctx := context.Background()
	// A snapshot named "KO: x" in the list payload is data, not a rejection.
	c, _ := newFake("OK\nID  TAG\n--  KO: weird name\n")
	if _, err := c.Snapshot(ctx, "list", ""); err != nil {
		t.Fatalf("payload containing KO: mid-line must not be an error: %v", err)
	}
	c, _ = newFake("some banner\nKO: no such snapshot")
	if _, err := c.Snapshot(ctx, "load", "x"); err == nil {
		t.Fatal("KO: at line start must still be an error")
	}
}
