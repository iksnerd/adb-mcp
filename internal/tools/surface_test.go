package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// listedTools starts a server with every tool registered and returns what a
// client sees from tools/list.
func listedTools(t *testing.T) []*mcp.Tool {
	t.Helper()
	ctx := context.Background()
	s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	Register(s)
	st, ct := mcp.NewInMemoryTransports()
	if _, err := s.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	res, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	return res.Tools
}

// adb_reverse takes an integer device_port. The shorthand "adb_reverse tcp:8081"
// reads like a call but is not one, and an agent that copies it passes a
// string the schema rejects.
func TestToolTextNeverUsesAdbReverseShorthand(t *testing.T) {
	for _, tool := range listedTools(t) {
		if strings.Contains(tool.Description, "adb_reverse tcp:") {
			t.Errorf("%s description uses the adb_reverse tcp: shorthand", tool.Name)
		}
		schema, _ := tool.InputSchema.(map[string]any)
		props, _ := schema["properties"].(map[string]any)
		for name, p := range props {
			if d, _ := p.(map[string]any)["description"].(string); strings.Contains(d, "adb_reverse tcp:") {
				t.Errorf("%s param %s uses the adb_reverse tcp: shorthand", tool.Name, name)
			}
		}
	}
}
