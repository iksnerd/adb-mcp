package gradle

import (
	"context"
	"regexp"
	"strings"
)

// assembleTaskRe matches a build-variant assemble task at the start of a line in
// `gradlew tasks` output, e.g. "assembleDebug - Assembles ..." or
// "assembleFreeRelease". The [A-Z] after "assemble" excludes the bare aggregate
// `assemble` task (followed by " - ...").
var assembleTaskRe = regexp.MustCompile(`(?m)^assemble([A-Z][A-Za-z0-9]*)\b`)

// ParseVariants extracts the buildable build-variant names from `gradlew tasks`
// output by reading its `assemble<Variant>` tasks. The variant is returned
// lower-camel (Gradle title-cases it in the task name, so assembleFreeDebug →
// freeDebug). The test-only aggregate tasks (`assembleAndroidTest`, and the
// per-variant `...AndroidTest`/`...UnitTest`) are skipped, since they build test
// APKs rather than a shippable variant. Pure string work, unit-tested directly.
func ParseVariants(tasksOutput string) []string {
	seen := map[string]bool{}
	var variants []string
	for _, m := range assembleTaskRe.FindAllStringSubmatch(tasksOutput, -1) {
		name := m[1]
		if strings.HasSuffix(name, "AndroidTest") || strings.HasSuffix(name, "UnitTest") {
			continue
		}
		variant := strings.ToLower(name[:1]) + name[1:]
		if !seen[variant] {
			seen[variant] = true
			variants = append(variants, variant)
		}
	}
	return variants
}

// ListVariants runs `gradlew tasks` in projectDir and parses out the buildable
// build variants. The raw output is returned too, so a caller can surface it
// when nothing parsed (e.g. project_dir points at the wrong module).
func ListVariants(ctx context.Context, projectDir string) (variants []string, rawOutput string, err error) {
	out, err := Gradle(ctx, projectDir, "tasks")
	if err != nil {
		return nil, out, err
	}
	return ParseVariants(out), out, nil
}
