package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/iksnerd/adb_mcp/internal/adb"
	"github.com/iksnerd/adb_mcp/internal/uiauto"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---- Arguments ----

type screenshotArgs struct {
	serialArg
	MaxDim *int `json:"max_dim,omitempty" jsonschema:"Max width/height of the returned image in pixels. Omit for the default 760; pass 0 (or a negative) to disable downscaling and get the full-resolution image."`
}

type describeUIArgs struct {
	serialArg
	Filter  string `json:"filter,omitempty" jsonschema:"What to include: 'auto' (default — elements with text, content_desc, resource_id, or clickable; identical-bounds label-less wrappers dropped), 'clickable' (tap targets only, the smallest view), or 'all' (every bounded node, unfiltered — use to PROVE an element is absent from the hierarchy)."`
	Query   string `json:"query,omitempty" jsonschema:"Case-insensitive substring to match against text, content_desc, and resource_id — return only matching elements. The cheap way to ask 'is X on this screen?'. Combine with filter='all' to prove absence definitively."`
	Compact *bool  `json:"compact,omitempty" jsonschema:"Return one line per element (center, bounds, flags, labels) instead of JSON — ~10x fewer tokens, same aiming information. Use for repeated look-drive loops and geometry work."`
}

type waitForTextArgs struct {
	serialArg
	Text     string `json:"text" jsonschema:"Text or content-description to wait for."`
	Partial  *bool  `json:"partial,omitempty" jsonschema:"Substring match instead of exact. Default true."`
	TimeoutS int    `json:"timeout_s,omitempty" jsonschema:"How long to wait, in seconds. Default 15."`
}

// ---- Handlers ----

func screenshot(ctx context.Context, in screenshotArgs) (*mcp.CallToolResult, error) {
	c, err := resolve(ctx, in.Serial)
	if err != nil {
		return nil, err
	}
	maxDim := 760 // default when omitted; an explicit 0/negative disables downscaling
	if in.MaxDim != nil {
		maxDim = *in.MaxDim
	}
	cap, err := c.CaptureScreen(ctx, maxDim)
	if err != nil {
		return nil, err
	}
	caption := fmt.Sprintf("Screenshot of %s (%dx%d).", c.Serial, cap.Width, cap.Height)
	if cap.AllBlack {
		caption += " " + blackCaptureNote(cap)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: caption},
			&mcp.ImageContent{Data: cap.PNG, MIMEType: "image/png"},
		},
	}, nil
}

// blackCaptureNote explains an all-black frame and points at describe_ui, so
// the caller doesn't misread "black" as "screen asleep" when it isn't. It ends
// with a compact machine-readable status the model can branch on.
func blackCaptureNote(c adb.ScreenCapture) string {
	var reason string
	switch {
	case c.SecureWindow:
		reason = "a FLAG_SECURE window is on screen — the OS blanks secure content (e.g. a native PIN pad) to black in screenshots"
	case c.ScreenOff:
		reason = "the display appears to be asleep/dozing"
	default:
		reason = fmt.Sprintf("the frame came back all black after %d attempt(s) — likely an intermittent capture glitch, not a real black screen", c.Attempts)
	}
	return fmt.Sprintf("WARNING: image is all black — %s. Fall back to describe_ui to read the UI (it works even when a screenshot is blanked). "+
		"status: {\"all_black\":true,\"secure_window\":%t,\"screen_off\":%t,\"attempts\":%d}",
		reason, c.SecureWindow, c.ScreenOff, c.Attempts)
}

func describeUI(ctx context.Context, in describeUIArgs) (*mcp.CallToolResult, error) {
	c, err := resolve(ctx, in.Serial)
	if err != nil {
		return nil, err
	}
	filter, err := uiauto.ParseUIFilter(in.Filter)
	if err != nil {
		return nil, err
	}
	snap, err := c.DescribeUI(ctx, filter)
	if err != nil {
		return nil, err
	}
	header := uiHeader(snap, filter)
	shown := snap.Elements
	if in.Query != "" {
		shown = uiauto.FilterByQuery(shown, in.Query)
		header += fmt.Sprintf("\n%d of %d element(s) match query %q:", len(shown), len(snap.Elements), in.Query)
	}
	var body string
	if boolOr(in.Compact, false) {
		body = compactUI(shown)
	} else {
		b, err := json.MarshalIndent(shown, "", "  ")
		if err != nil {
			return nil, err
		}
		body = string(b)
	}
	return &mcp.CallToolResult{Content: []mcp.Content{
		&mcp.TextContent{Text: header + "\n" + body},
	}}, nil
}

// compactUI renders elements one per line — the same aiming information as the
// JSON form at a fraction of the tokens.
func compactUI(elems []uiauto.Element) string {
	if len(elems) == 0 {
		return "(no elements)"
	}
	var b strings.Builder
	for i := range elems {
		e := &elems[i]
		fmt.Fprintf(&b, "(%d,%d) [%d,%d][%d,%d]", e.Center.X, e.Center.Y, e.Bounds.X1, e.Bounds.Y1, e.Bounds.X2, e.Bounds.Y2)
		if e.Clickable {
			b.WriteString(" clickable")
		}
		if e.Focused {
			b.WriteString(" focused")
		}
		if e.Text != "" {
			fmt.Fprintf(&b, " text:%q", e.Text)
		}
		if e.Desc != "" {
			fmt.Fprintf(&b, " desc:%q", e.Desc)
		}
		if e.ResourceID != "" {
			fmt.Fprintf(&b, " id:%s", e.ResourceID)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// uiHeader gives the two facts needed to trust (or distrust) the element list:
// whose window the hierarchy belongs to, and how much the filter hid.
func uiHeader(snap adb.UISnapshot, filter uiauto.UIFilter) string {
	var b strings.Builder
	if snap.TopWindow != "" {
		fmt.Fprintf(&b, "top window: %s", snap.TopWindow)
		if strings.Contains(snap.TopWindow, "com.android.systemui") ||
			strings.Contains(snap.TopWindow, "com.google.android.permissioncontroller") {
			b.WriteString(" — a SYSTEM overlay (biometric prompt / permission dialog / shade) has focus; the elements below belong to IT, and the app underneath is occluded")
		}
		b.WriteString("\n")
	}
	if snap.Hidden > 0 {
		fmt.Fprintf(&b, "%d node(s) hidden by filter=%q — absence below does NOT prove an element is missing; re-run with filter=\"all\" to see the raw hierarchy\n", snap.Hidden, string(filter))
	} else {
		fmt.Fprintf(&b, "0 nodes hidden (filter=%q) — this is the complete bounded hierarchy; absence below is trustworthy\n", string(filter))
	}
	fmt.Fprintf(&b, "%d element(s):", len(snap.Elements))
	return b.String()
}

func waitForText(ctx context.Context, in waitForTextArgs) (*mcp.CallToolResult, error) {
	c, err := resolve(ctx, in.Serial)
	if err != nil {
		return nil, err
	}
	timeout := time.Duration(in.TimeoutS) * time.Second
	e, err := c.WaitForText(ctx, in.Text, boolOr(in.Partial, true), timeout)
	if err != nil {
		return nil, err
	}
	return jsonResult(e)
}
