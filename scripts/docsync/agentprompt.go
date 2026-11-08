package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The "paste this to your agent" setup prompt has one source,
// docs/agent-setup-prompt.txt. The website reads that file from GitHub, and the
// README carries a copy between two markers; syncAgentPrompt keeps the copy equal.
const (
	agentPromptPath = "docs/agent-setup-prompt.txt"
	agentStart      = "<!-- agent-setup:start (generated from docs/agent-setup-prompt.txt by make docs) -->\n"
	agentEnd        = "<!-- agent-setup:end -->"
)

// applyAgentPrompt returns readme with the marked block rebuilt from prompt.
func applyAgentPrompt(readme, prompt string) (string, error) {
	i := strings.Index(readme, agentStart)
	j := strings.Index(readme, agentEnd)
	if i < 0 || j < i {
		return "", fmt.Errorf("README.md has no %q ... %q block", strings.TrimSpace(agentStart), agentEnd)
	}
	block := agentStart + "````text\n" + strings.TrimRight(prompt, "\n") + "\n````\n"
	return readme[:i] + block + readme[j:], nil
}

// syncAgentPrompt writes the README block, or in check mode reports it when stale.
func syncAgentPrompt(root string, check bool) (stale []string) {
	prompt, err := os.ReadFile(filepath.Join(root, agentPromptPath))
	if err != nil {
		fail(err)
	}
	path := filepath.Join(root, "README.md")
	before, err := os.ReadFile(path)
	if err != nil {
		fail(err)
	}
	after, err := applyAgentPrompt(string(before), string(prompt))
	if err != nil {
		fail(err)
	}
	if after == string(before) {
		return nil
	}
	if check {
		return []string{"README.md (agent setup prompt)"}
	}
	if err := os.WriteFile(path, []byte(after), 0o644); err != nil {
		fail(err)
	}
	fmt.Println("updated README.md (agent setup prompt)")
	return nil
}
