package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type panicArgs struct{}

// go-sdk does not recover a panicking handler, so one bad tool call would end
// the process and skip the shutdown cleanup. add() turns it into a tool error.
func TestAddTurnsAHandlerPanicIntoAToolError(t *testing.T) {
	ctx := context.Background()
	s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	add(s, "boom_tool", "panics", func(context.Context, panicArgs) (*mcp.CallToolResult, error) {
		panic("kaboom")
	})
	add(s, "fine_tool", "works", func(context.Context, panicArgs) (*mcp.CallToolResult, error) {
		return text("still alive"), nil
	})
	st, ct := mcp.NewInMemoryTransports()
	if _, err := s.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	session, err := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "boom_tool", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("a panic must come back as a tool result, got protocol error: %v", err)
	}
	if !res.IsError {
		t.Error("result should be marked isError")
	}
	var body string
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			body += tc.Text
		}
	}
	if !strings.Contains(body, "kaboom") || !strings.Contains(body, "boom_tool") {
		t.Errorf("result should carry the panic text and tool name, got %q", body)
	}

	// The server survives and keeps serving.
	if res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "fine_tool", Arguments: map[string]any{}}); err != nil || res.IsError {
		t.Fatalf("server not usable after a panic: %v %v", res, err)
	}
}
