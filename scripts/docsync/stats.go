package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// docs/stats.json is the machine-readable version of the numbers the docs state
// in prose. The website reads it, the way it reads the star count, instead of
// matching a regex against a sentence in TOOLS.md: a reworded sentence made that
// match fail without any error, and the page then showed a hand-typed fallback.
// Here the numbers are generated from the code and `make check` fails when the
// committed file differs, so they cannot drift.
const statsPath = "docs/stats.json"

// reHintRow matches one row of the hints table in internal/tools/annotations.go:
// the effect constructor, then any .idempotent() / .network() modifiers.
var reHintRow = regexp.MustCompile(`(?m)^\t"[a-z_]+":\s+(observe|change|destroy)\(\)((?:\.\w+\(\))*)`)

type hintCounts struct{ readOnly, additive, destructive, openWorld int }

// countHints tallies the hints table by effect and counts the rows that can
// reach beyond the device (.network()).
func countHints(src string) hintCounts {
	var h hintCounts
	for _, m := range reHintRow.FindAllStringSubmatch(src, -1) {
		switch m[1] {
		case "observe":
			h.readOnly++
		case "change":
			h.additive++
		case "destroy":
			h.destructive++
		}
		if strings.Contains(m[2], ".network()") {
			h.openWorld++
		}
	}
	return h
}

type statsFile struct {
	Version     string `json:"version"`
	Tools       int    `json:"tools"`
	Guides      int    `json:"guides"`
	ReadOnly    int    `json:"read_only"`
	Additive    int    `json:"additive"`
	Destructive int    `json:"destructive"`
	OpenWorld   int    `json:"open_world"`
}

// renderStats returns the exact bytes of docs/stats.json. Field order is the
// struct's, so the output is deterministic and the check is a plain comparison.
func renderStats(c counts, h hintCounts) string {
	b, err := json.MarshalIndent(statsFile{
		Version: c.version, Tools: c.tools, Guides: c.guides,
		ReadOnly: h.readOnly, Additive: h.additive, Destructive: h.destructive, OpenWorld: h.openWorld,
	}, "", "  ")
	if err != nil {
		fail(err)
	}
	return string(b) + "\n"
}

// syncStats writes docs/stats.json, or in check mode reports it when stale.
// It also refuses a hints table that does not account for every registered
// tool, because that would publish a wrong count.
func syncStats(root string, c counts, check bool) (stale []string) {
	table, err := os.ReadFile(filepath.Join(root, "internal/tools/annotations.go"))
	if err != nil {
		fail(err)
	}
	h := countHints(string(table))
	if rows := h.readOnly + h.additive + h.destructive; rows != c.tools {
		fail(fmt.Errorf("internal/tools/annotations.go has %d hint rows for %d registered tools; the scan or the table is out of step", rows, c.tools))
	}
	want := renderStats(c, h)
	path := filepath.Join(root, statsPath)
	have, err := os.ReadFile(path)
	if err == nil && string(have) == want {
		return nil
	}
	if check {
		return []string{statsPath}
	}
	if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
		fail(err)
	}
	fmt.Println("updated", statsPath)
	return nil
}
