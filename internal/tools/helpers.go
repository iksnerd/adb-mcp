package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/iksnerd/adb-mcp/internal/adb"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// serialArg is embedded by every device-targeted tool's arguments.
type serialArg struct {
	Serial string `json:"serial,omitempty" jsonschema:"Target device serial (adb -s). Optional when exactly one device is attached."`
}

// projectDirArg is embedded by every Gradle-project-targeted tool's arguments.
type projectDirArg struct {
	ProjectDir string `json:"project_dir,omitempty" jsonschema:"Path to the Android project root containing the Gradle wrapper (gradlew). Optional if session_set_defaults has pinned a project_dir for this session."`
}

// text formats a plain-text tool result.
func text(format string, args ...any) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(format, args...)}}}
}

// jsonResult renders v as indented JSON in a tool result.
func jsonResult(v any) (*mcp.CallToolResult, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil
}

// resolve turns an optional serial argument into a ready adb Client: it applies
// the single-device default (adb.ResolveSerial) and returns a client bound
// to that device. Handlers read c.Serial for messages and call c.Method(...).
func resolve(ctx context.Context, serial string) (*adb.Client, error) {
	if strings.TrimSpace(serial) == "" {
		_, serial = getSessionDefaults()
	}
	s, err := adb.ResolveSerial(ctx, serial)
	if err != nil {
		return nil, err
	}
	return adb.New(s), nil
}

// boolOr returns *p, or def when p is nil (for optional bool arguments).
func boolOr(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

// requireNonEmpty rejects a blank required string argument. The schema's
// `required` only checks that the key is present, so "" or "  " reaches the
// handler and would otherwise be passed straight to adb (`adb uninstall ""`).
func requireNonEmpty(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required and must not be empty", name)
	}
	return nil
}

// Handler-side defaults for optional arguments. Each is also stated in the
// matching schema description, and defaults_test.go fails if the two drift.
const (
	defaultPartial        = true // tap_on_text / tap_element / wait_for_text partial
	defaultCaptureFinal   = true // run_sequence capture_final
	defaultStopLogcatTail = 500  // stop_logcat_capture tail
	defaultScreenshotDim  = 760  // screenshot max_dim
)

// partialOrDefault resolves an optional "partial" (substring match) argument.
func partialOrDefault(p *bool) bool { return boolOr(p, defaultPartial) }
