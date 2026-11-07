package adb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/iksnerd/adb-mcp/internal/concurrent"
	"github.com/iksnerd/adb-mcp/internal/uiauto"
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
	// A per-call device path: two overlapping dumps (or two servers) must not
	// read each other's half-written file.
	remote := "/sdcard/aemcp-ui-" + randToken() + ".xml"
	defer func() { _, _ = c.adb(context.WithoutCancel(ctx), "shell", "rm", "-f", remote) }()
	var lastErr error
	for attempt := range 2 {
		if attempt > 0 {
			time.Sleep(1500 * time.Millisecond)
		}
		// uiautomator can exit 0 without writing a file ("could not get idle
		// state"); clear first so a leftover can never pass as the current tree.
		_, _ = c.adb(ctx, "shell", "rm", "-f", remote)
		dumpOut, err := c.adb(ctx, "shell", "uiautomator", "dump", remote)
		if err != nil {
			lastErr = err
			continue
		}
		if strings.Contains(dumpOut, "ERROR") {
			lastErr = fmt.Errorf("uiautomator: %s", strings.TrimSpace(dumpOut))
			continue
		}
		out, err := c.adbBytes(ctx, "shell", "cat", remote)
		if err != nil {
			lastErr = err
			continue
		}
		xmlStr := string(out)
		if strings.TrimSpace(xmlStr) == "" {
			lastErr = fmt.Errorf("uiautomator dump wrote no file")
			continue
		}
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

// randToken returns a short random hex string for per-call device file names.
func randToken() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
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
	// TopWindow doesn't read the hierarchy dump and describeSettled doesn't read
	// TopWindow, so run them concurrently — describeSettled's own settle-loop
	// (one to three dumps, 500ms-1.5s+ apart) dominates either way, so this just
	// folds TopWindow's round-trip into that wait instead of paying for it after.
	var elems []uiauto.Element
	var hidden int
	var err error
	var top string
	concurrent.RunAll(
		func() { elems, hidden, err = c.describeSettled(ctx, filter) },
		// Best-effort: an unreadable focus probe should not fail the whole read.
		func() { top, _ = c.TopWindow(ctx) },
	)
	if err != nil {
		return UISnapshot{}, err
	}
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
func (c *Client) WaitForText(ctx context.Context, query string, partial bool, timeout time.Duration, scroll bool) (uiauto.Element, error) {
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
			if !scroll {
				return uiauto.Element{}, fmt.Errorf("text %q did not appear within %s — this only polls the current viewport; Android omits off-screen ScrollView content from the accessibility tree, so if the text may be scrolled out of view, retry with scroll=true before concluding it's absent", query, timeout)
			}
			return uiauto.Element{}, fmt.Errorf("text %q did not appear within %s (scrolled while waiting)", query, timeout)
		}
		if scroll {
			// Android's accessibility tree commonly omits content outside the
			// viewport. A modest upward swipe lets an opt-in wait discover rows
			// in a ScrollView without changing the default polling semantics.
			if w, h := c.displaySize(ctx); w > 0 && h > 0 {
				_ = c.Swipe(ctx, w/2, h*3/4, w/2, h/4, 250)
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// displaySize returns the current logical screen size in pixels. An active
// `wm size` override wins over the physical size (input coordinates follow the
// override), and the axes are swapped when the display is rotated to landscape
// (best effort: read from `dumpsys input`'s SurfaceOrientation, left unswapped
// when it can't be read).
func (c *Client) displaySize(ctx context.Context) (int, int) {
	out, err := c.adb(ctx, "shell", "wm", "size")
	if err != nil {
		return 0, 0
	}
	var w, h int
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		i := strings.LastIndex(line, ":")
		if i < 0 {
			continue
		}
		parts := strings.Split(strings.TrimSpace(line[i+1:]), "x")
		if len(parts) != 2 {
			continue
		}
		pw, e1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		ph, e2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		if e1 != nil || e2 != nil {
			continue
		}
		w, h = pw, ph
		if strings.HasPrefix(line, "Override") {
			break
		}
	}
	if w > 0 && h > 0 && c.landscape(ctx) {
		w, h = h, w
	}
	return w, h
}

// landscape reports whether the display is rotated 90/270 degrees.
func (c *Client) landscape(ctx context.Context) bool {
	out, err := c.adb(ctx, "shell", "dumpsys", "input")
	if err != nil {
		return false
	}
	_, rest, ok := strings.Cut(out, "SurfaceOrientation:")
	if !ok {
		return false
	}
	rest = strings.TrimSpace(rest)
	return strings.HasPrefix(rest, "1") || strings.HasPrefix(rest, "3")
}
