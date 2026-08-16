package tools

import (
	"context"
	"fmt"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// sessionDefaults holds per-process defaults (project_dir, serial) that
// device/Gradle tools fall back to when a call omits them — pinning a
// multi-module or multi-flavor project's project_dir, or a serial among
// several attached devices, once instead of on every call. The server runs
// one process per stdio connection (see cmd/adb-mcp/main.go), so a package
// -level store is exactly session-scoped: it never outlives or crosses the
// client connection it belongs to.
var sessionDefaults struct {
	mu         sync.Mutex
	projectDir string
	serial     string
}

func getSessionDefaults() (projectDir, serial string) {
	sessionDefaults.mu.Lock()
	defer sessionDefaults.mu.Unlock()
	return sessionDefaults.projectDir, sessionDefaults.serial
}

// resolveProjectDir returns explicit, or the session default project_dir if
// explicit is empty, or an error naming both ways to supply one.
func resolveProjectDir(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	dir, _ := getSessionDefaults()
	if dir != "" {
		return dir, nil
	}
	return "", fmt.Errorf("project_dir is required (pass it directly, or call session_set_defaults once to pin it for the rest of this session)")
}

// ---- Arguments ----

type sessionSetDefaultsArgs struct {
	ProjectDir string `json:"project_dir,omitempty" jsonschema:"Default Android project root to use whenever a tool call omits project_dir. Leave empty to leave the current default unchanged."`
	Serial     string `json:"serial,omitempty" jsonschema:"Default device serial to use whenever a tool call omits serial. Leave empty to leave the current default unchanged."`
}

type sessionShowDefaultsArgs struct{}

type sessionClearDefaultsArgs struct{}

// ---- Handlers ----

func sessionSetDefaults(_ context.Context, in sessionSetDefaultsArgs) (*mcp.CallToolResult, error) {
	sessionDefaults.mu.Lock()
	if in.ProjectDir != "" {
		sessionDefaults.projectDir = in.ProjectDir
	}
	if in.Serial != "" {
		sessionDefaults.serial = in.Serial
	}
	dir, serial := sessionDefaults.projectDir, sessionDefaults.serial
	sessionDefaults.mu.Unlock()
	return text("%s", formatSessionDefaults(dir, serial)), nil
}

func sessionShowDefaults(_ context.Context, _ sessionShowDefaultsArgs) (*mcp.CallToolResult, error) {
	dir, serial := getSessionDefaults()
	return text("%s", formatSessionDefaults(dir, serial)), nil
}

func sessionClearDefaults(_ context.Context, _ sessionClearDefaultsArgs) (*mcp.CallToolResult, error) {
	sessionDefaults.mu.Lock()
	sessionDefaults.projectDir = ""
	sessionDefaults.serial = ""
	sessionDefaults.mu.Unlock()
	return text("Session defaults cleared."), nil
}

func formatSessionDefaults(dir, serial string) string {
	if dir == "" && serial == "" {
		return "No session defaults set. project_dir and serial must be passed explicitly to every call, or set here."
	}
	msg := "Session defaults:"
	if dir != "" {
		msg += fmt.Sprintf("\n  project_dir: %s", dir)
	} else {
		msg += "\n  project_dir: (not set — required on every call)"
	}
	if serial != "" {
		msg += fmt.Sprintf("\n  serial: %s", serial)
	} else {
		msg += "\n  serial: (not set — falls back to the single-attached-device default)"
	}
	return msg
}
