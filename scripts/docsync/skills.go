package main

import (
	"fmt"
	"strings"
)

// syncSkill returns skill with everything after its front matter replaced by
// guide. The front matter stays hand-written: it is the trigger text, and it
// decides whether a client loads the skill at all. The body is the guide,
// verbatim, so the two cannot drift.
//
// The body has to be the guide itself, not a pointer to it. Claude Desktop does
// not read MCP resources, so a skill that says "read the android://guide/x
// resource" gives a Desktop user only its one-line description.
func syncSkill(skill, guide string) (string, error) {
	const fence = "---\n"
	if !strings.HasPrefix(skill, fence) {
		return "", fmt.Errorf("no front matter (a SKILL.md starts with %q)", strings.TrimSpace(fence))
	}
	end := strings.Index(skill[len(fence):], "\n"+fence)
	if end < 0 {
		return "", fmt.Errorf("front matter is not closed with %q", strings.TrimSpace(fence))
	}
	head := skill[:len(fence)+end+len("\n"+fence)]
	return head + "\n" + strings.TrimRight(guide, "\n") + "\n", nil
}
