package main

import (
	"strings"
	"testing"
)

func TestApplyAgentPromptRebuildsTheBlock(t *testing.T) {
	readme := "intro\n" + agentStart + "````text\nold\n````\n" + agentEnd + "\nrest\n"
	got, err := applyAgentPrompt(readme, "new line\nsecond\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "intro\n" + agentStart + "````text\nnew line\nsecond\n````\n" + agentEnd + "\nrest\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	again, _ := applyAgentPrompt(got, "new line\nsecond\n")
	if again != got {
		t.Error("applying twice changed the README: the rewrite is not stable")
	}
}

func TestApplyAgentPromptRefusesAReadmeWithoutMarkers(t *testing.T) {
	if _, err := applyAgentPrompt("no markers here", "x"); err == nil || !strings.Contains(err.Error(), "agent-setup") {
		t.Errorf("want an error naming the markers, got %v", err)
	}
}
