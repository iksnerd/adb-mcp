package gradle

import (
	"context"
	"regexp"
)

// projectPathRe matches a module line in `gradlew projects` output, e.g.
//
//	+--- Project ':app'
//	\--- Project ':feature:login'
//
// The tree-drawing prefix (+---, \---, |, spaces) precedes "Project '<path>'".
// The root project prints as "Root project '<name>'" and is intentionally not
// matched — it has no ':' Gradle path to build against.
var projectPathRe = regexp.MustCompile(`Project '(:[^']*)'`)

// ParseProjects extracts the sub-project (module) Gradle paths from
// `gradlew projects` output — e.g. [":app", ":core", ":feature:login"] — in
// declaration order, de-duplicated. The root project is skipped (it has no
// ':path'). Pure string work, unit-tested directly.
func ParseProjects(projectsOutput string) []string {
	seen := map[string]bool{}
	var paths []string
	for _, m := range projectPathRe.FindAllStringSubmatch(projectsOutput, -1) {
		p := m[1]
		if !seen[p] {
			seen[p] = true
			paths = append(paths, p)
		}
	}
	return paths
}

// ListProjects runs `gradlew projects` in projectDir and parses out the module
// paths. The raw output is returned too, so a caller can surface it when nothing
// parsed (e.g. a single-module build with no sub-projects).
func ListProjects(ctx context.Context, projectDir string) (paths []string, rawOutput string, err error) {
	out, err := Gradle(ctx, projectDir, "projects")
	if err != nil {
		return nil, out, err
	}
	return ParseProjects(out), out, nil
}
