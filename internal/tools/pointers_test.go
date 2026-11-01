package tools

import (
	"strings"
	"testing"
)

// Claude Desktop does not read MCP resources, so a tool description or the
// server instructions that tells the model to read android://guide/<name> sends
// it somewhere it cannot go. The same guides ship as skills, so every mention
// of a guide's resource URI must also say it is available as a skill.
func TestGuidePointersNameTheSkillToo(t *testing.T) {
	check := func(where, text string) {
		if strings.Contains(text, "android://guide/") && !strings.Contains(text, "skill") {
			t.Errorf("%s points at an android://guide/ resource without saying it is also a skill:\n%s", where, text)
		}
	}
	for _, tool := range listedTools(t) {
		check("tool "+tool.Name, tool.Description)
	}
	check("server instructions", Instructions)
}
