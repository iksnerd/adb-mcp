package adb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/iksnerd/adb_mcp/internal/uiauto"
)

// dumpOnce performs a single uiautomator dump + parse with the default filter,
// for callers that only need elements for text lookup.
func (c *Client) dumpOnce(ctx context.Context) ([]uiauto.Element, error) {
	elems, _, err := c.dumpFiltered(ctx, uiauto.FilterAuto)
	return elems, err
}

// dumpFiltered performs a single uiautomator dump + parse, retrying once on the
// transient "could not get idle state" error that occurs mid-animation. It
// also reports how many bounded nodes the filter hid.
func (c *Client) dumpFiltered(ctx context.Context, filter uiauto.UIFilter) ([]uiauto.Element, int, error) {
	const remote = "/sdcard/window_dump.xml"
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			time.Sleep(1500 * time.Millisecond)
		}
		if _, err := c.adb(ctx, "shell", "uiautomator", "dump", remote); err != nil {
			lastErr = err
			continue
		}
		out, err := c.adbBytes(ctx, "shell", "cat", remote)
		if err != nil {
			lastErr = err
			continue
		}
		xmlStr := string(out)
		if i := strings.Index(xmlStr, "<?xml"); i > 0 {
			xmlStr = xmlStr[i:]
		}
		elems, hidden, err := uiauto.ParseHierarchyFiltered(xmlStr, filter)
		if err != nil {
			lastErr = err
			continue
		}
		return elems, hidden, nil
	}
	return nil, 0, fmt.Errorf("uiautomator dump failed (device busy?): %w", lastErr)
}

// UISnapshot is a settled hierarchy read plus the context needed to trust it:
// which window actually has focus (the hierarchy may belong to a system
// overlay, not the app under test) and how many nodes the filter hid (so
// "absent from Elements" is distinguishable from "filtered out").
type UISnapshot struct {
	TopWindow string
	Elements  []uiauto.Element
	Hidden    int
}

// DescribeUI reads the UI hierarchy and returns a *settled* snapshot with
// true-pixel bounds and precomputed centers. uiautomator can hand back a STALE
// tree mid-refresh/animation (e.g. a list's empty-state while it is really
// populating). To guard against that, DescribeUI dumps twice ~500ms apart; if
// the snapshots differ the UI is still changing, so it takes one more after a
// longer wait and returns that freshest snapshot.
//
// Note: content drawn on a canvas (React Native / Flutter / Skia PIN pads) does
// not appear in the hierarchy at all — no amount of settling surfaces it. For
// those, use a screenshot to read state and tap by coordinate (see enter_pin's
// grid/coords options for custom pads).
func (c *Client) DescribeUI(ctx context.Context, filter uiauto.UIFilter) (UISnapshot, error) {
	elems, hidden, err := c.describeSettled(ctx, filter)
	if err != nil {
		return UISnapshot{}, err
	}
	// Best-effort: an unreadable focus probe should not fail the whole read.
	top, _ := c.TopWindow(ctx)
	return UISnapshot{TopWindow: top, Elements: elems, Hidden: hidden}, nil
}

func (c *Client) describeSettled(ctx context.Context, filter uiauto.UIFilter) ([]uiauto.Element, int, error) {
	first, firstHidden, err := c.dumpFiltered(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	time.Sleep(500 * time.Millisecond)
	second, secondHidden, err := c.dumpFiltered(ctx, filter)
	if err != nil {
		return first, firstHidden, nil // a failed re-dump is non-fatal; return what we have
	}
	if signature(first) == signature(second) {
		return second, secondHidden, nil
	}
	time.Sleep(1000 * time.Millisecond)
	third, thirdHidden, err := c.dumpFiltered(ctx, filter)
	if err != nil {
		return second, secondHidden, nil
	}
	return third, thirdHidden, nil
}

// UISignature returns a cheap fingerprint of the current hierarchy. Take one
// before and one after an action to learn whether the action had any visible
// effect (see the verify_change option on press_key/tap).
func (c *Client) UISignature(ctx context.Context) (string, error) {
	elems, err := c.dumpOnce(ctx)
	if err != nil {
		return "", err
	}
	return signature(elems), nil
}

// signature is a cheap fingerprint of a hierarchy used to detect whether two
// consecutive dumps describe the same (settled) screen.
func signature(elems []uiauto.Element) string {
	var b strings.Builder
	for i := range elems {
		e := &elems[i]
		fmt.Fprintf(&b, "%s|%s|%s|%d,%d;", e.Text, e.Desc, e.ResourceID, e.Bounds.X1, e.Bounds.Y1)
	}
	return b.String()
}

// WaitForText polls the UI hierarchy until an element matching query (by text or
// content-description) appears, or timeout elapses. It is the reliable
// alternative to a manual sleep-then-screenshot loop after an async action.
func (c *Client) WaitForText(ctx context.Context, query string, partial bool, timeout time.Duration) (uiauto.Element, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for {
		if err := ctx.Err(); err != nil {
			return uiauto.Element{}, err
		}
		elems, err := c.dumpOnce(ctx)
		if err == nil {
			if e, ok := uiauto.FindByText(elems, query, partial); ok {
				return e, nil
			}
		}
		if time.Now().After(deadline) {
			return uiauto.Element{}, fmt.Errorf("text %q did not appear within %s", query, timeout)
		}
		time.Sleep(500 * time.Millisecond)
	}
}
