package selfupdate

import "testing"

func TestPluginManaged(t *testing.T) {
	cases := map[string]bool{
		"/home/u/.claude/plugins/data/adb-mcp-x/adb-mcp/0.24.6/linux-amd64/adb-mcp": true,
		"/Users/u/.claude/plugins/data/p/adb-mcp/1.2.3/darwin-arm64/adb-mcp":        true,
		"C:/Users/u/data/adb-mcp/0.24.6/windows-amd64/adb-mcp.exe":                  true,
		"/Users/u/.local/bin/adb-mcp":                                               false,
		"/Users/u/go/bin/adb-mcp":                                                   false,
		"/Users/u/src/adb-mcp/bin/adb-mcp":                                          false,
		"/Users/u/src/adb-mcp/1.2.3/notatarget/adb-mcp":                             false,
	}
	for exe, want := range cases {
		if got := pluginManaged(exe); got != want {
			t.Errorf("pluginManaged(%q) = %v, want %v", exe, got, want)
		}
	}
}
