package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/iksnerd/adb-mcp/internal/adb"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---- Arguments ----

type logcatArgs struct {
	serialArg
	Lines    int      `json:"lines,omitempty" jsonschema:"Number of recent lines to dump. Default 400. Ignored when since is given."`
	Since    string   `json:"since,omitempty" jsonschema:"Time window instead of a line count: only lines from the last e.g. \"2m\", \"90s\", \"1h30m\" (device clock). The right axis when the report is \"I just hit an error\" — on a chatty emulator 400 lines can span under ten seconds."`
	Filter   string   `json:"filter,omitempty" jsonschema:"Case-insensitive substring to keep (e.g. an app tag or \"Exception\")."`
	Priority string   `json:"priority,omitempty" jsonschema:"Minimum priority to keep: V, D, I, W, E, or F (matches adb's own \"*:E\"-style filter — E keeps Error and Fatal). Omit for no priority filtering."`
	Tags     []string `json:"tags,omitempty" jsonschema:"Keep only lines whose log tag contains one of these (case-insensitive, OR'd), e.g. [\"SessionStore\",\"AuthModule\"]. Omit for no tag filtering."`
	Redact   bool     `json:"redact,omitempty" jsonschema:"Mask common secrets (token, password, authorization, api key, secret) before returning log lines. Recommended for payment/auth SDKs and debug builds."`
}

type startLogcatArgs struct {
	serialArg
	Clear *bool `json:"clear,omitempty" jsonschema:"Clear the logcat buffer before capturing. Default true."`
}

type stopLogcatArgs struct {
	serialArg
	Filter   string   `json:"filter,omitempty" jsonschema:"Case-insensitive substring to keep."`
	Priority string   `json:"priority,omitempty" jsonschema:"Minimum priority to keep: V, D, I, W, E, or F."`
	Tags     []string `json:"tags,omitempty" jsonschema:"Keep only lines whose log tag contains one of these (case-insensitive, OR'd)."`
	Redact   bool     `json:"redact,omitempty" jsonschema:"Mask common secrets (token, password, authorization, api key, secret) before returning captured lines."`
	Tail     int      `json:"tail,omitempty" jsonschema:"Keep only the last N lines after filtering (the most recent, where a crash usually is). Default 500; pass a larger number for more, or a huge one to effectively disable the cap."`
}

type stopRecordArgs struct {
	serialArg
	LocalPath string `json:"local_path" jsonschema:"Local path to save the pulled mp4, e.g. /tmp/rec.mp4."`
}

// ---- Handlers ----

func logcat(ctx context.Context, in logcatArgs) (*mcp.CallToolResult, error) {
	c, err := resolve(ctx, in.Serial)
	if err != nil {
		return nil, err
	}
	out, err := c.Logcat(ctx, in.Lines, in.Since, adb.LogFilter{Substring: in.Filter, Priority: in.Priority, Tags: in.Tags, Redact: in.Redact})
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return text("(no matching log lines — if you're chasing a crash that already happened, it may have scrolled out of the ring buffer: wrap the repro in start_logcat_capture/stop_logcat_capture, or use last_crash for a fatal that hit the DropBox)"), nil
	}
	return text("%s", out), nil
}

func clearLogcat(ctx context.Context, in serialArg) (*mcp.CallToolResult, error) {
	c, err := resolve(ctx, in.Serial)
	if err != nil {
		return nil, err
	}
	if err := c.ClearLogcat(ctx); err != nil {
		return nil, err
	}
	return text("Logcat buffer cleared for %s — the next logcat read contains only lines logged from now on.", c.Serial), nil
}

func startLogcatCapture(ctx context.Context, in startLogcatArgs) (*mcp.CallToolResult, error) {
	c, err := resolve(ctx, in.Serial)
	if err != nil {
		return nil, err
	}
	if err := c.StartLogcatCapture(ctx, boolOr(in.Clear, true)); err != nil {
		return nil, err
	}
	return text("Logcat capture started for %s. Drive your flow, then stop_logcat_capture.", c.Serial), nil
}

func stopLogcatCapture(ctx context.Context, in stopLogcatArgs) (*mcp.CallToolResult, error) {
	c, err := resolve(ctx, in.Serial)
	if err != nil {
		return nil, err
	}
	out, err := c.StopLogcatCapture(adb.LogFilter{Substring: in.Filter, Priority: in.Priority, Tags: in.Tags, Redact: in.Redact})
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return text("(capture stopped; no matching lines)"), nil
	}
	return text("%s", tailLines(out, tailOrDefault(in.Tail))), nil
}

// tailOrDefault caps a long capture at the default so it doesn't blow the
// token budget; a larger explicit tail lifts the cap.
func tailOrDefault(n int) int {
	if n <= 0 {
		return defaultStopLogcatTail
	}
	return n
}

func startScreenRecord(ctx context.Context, in serialArg) (*mcp.CallToolResult, error) {
	c, err := resolve(ctx, in.Serial)
	if err != nil {
		return nil, err
	}
	if err := c.StartScreenRecord(ctx); err != nil {
		return nil, err
	}
	msg := fmt.Sprintf("Recording %s (max ~180s). Drive your flow, then stop_screen_record.", c.Serial)
	if !c.DisplayAwake(ctx) {
		// An asleep display sends no frames, so the recording would come out empty.
		msg += " Warning: the display is asleep or off, so this recording will be empty. Call stay_awake with enabled=true (or wake the device), then stop_screen_record and record again."
	}
	return text("%s", msg), nil
}

func stopScreenRecord(ctx context.Context, in stopRecordArgs) (*mcp.CallToolResult, error) {
	if err := requireNonEmpty("local_path", in.LocalPath); err != nil {
		return nil, err
	}
	c, err := resolve(ctx, in.Serial)
	if err != nil {
		return nil, err
	}
	path, err := c.StopScreenRecord(ctx, in.LocalPath)
	if err != nil {
		return nil, err
	}
	return text("Saved recording to %s.", path), nil
}
