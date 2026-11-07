package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The adb_reverse remediation must name the Metro port the caller asked for,
// not a hardcoded 8081.
func TestLaunchDevClientHintUsesActualPort(t *testing.T) {
	for _, tc := range []struct {
		port int
		want string
	}{{0, "device_port: 8081"}, {19000, "device_port: 19000"}} {
		fakeAdb(t, "")
		res, err := launchDevClient(context.Background(), launchDevClientArgs{Scheme: "myapp", Port: tc.port})
		if err != nil {
			t.Fatalf("port %d: %v", tc.port, err)
		}
		got := res.Content[0].(*mcp.TextContent).Text
		if !strings.Contains(got, tc.want) {
			t.Errorf("port %d: %q lacks %q", tc.port, got, tc.want)
		}
		if tc.port != 0 && strings.Contains(got, "8081") {
			t.Errorf("port %d: hint still mentions 8081: %q", tc.port, got)
		}
	}
	if got := metroUnreachableHint(19000); !strings.Contains(got, "device_port: 19000") {
		t.Errorf("metroUnreachableHint(19000) = %q", got)
	}
}
