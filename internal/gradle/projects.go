package gradle

import (
	"context"
	"fmt"
	"regexp"
	"strings"
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

// secretPropertyKeyRe matches property-key substrings that commonly carry
// secrets. Gradle's stock `properties` task dumps a module's ENTIRE effective
// property set, including credentials injected via ~/.gradle/gradle.properties
// or env vars for private Maven repo auth — indistinguishable in the raw
// output from harmless entries like compileSdkVersion. Field report:
// council-hub android-mcp-papercuts #019fdd06.
var secretPropertyKeyRe = regexp.MustCompile(`(?i)(pass|secret|token|key|credential)`)

// propertyLineRe matches one "key: value" line from gradlew's properties task
// output, e.g. "NEXUS_PASS: hunter2".
var propertyLineRe = regexp.MustCompile(`^([A-Za-z_][\w.]*): (.*)$`)

// redactSecretProperties masks the value of any property line whose key looks
// secret-shaped, so a properties dump doesn't leak credentials into an agent
// transcript by default. Pure string work, unit-tested directly.
func redactSecretProperties(output string) string {
	lines := strings.Split(output, "\n")
	for i, line := range lines {
		m := propertyLineRe.FindStringSubmatch(line)
		if m != nil && secretPropertyKeyRe.MatchString(m[1]) {
			lines[i] = m[1] + ": ***redacted***"
		}
	}
	return strings.Join(lines, "\n")
}

// ListProjectProperties runs Gradle's standard properties task for one module
// and redacts any secret-shaped property values before returning. module is a
// Gradle path such as :app or :feature:login.
func ListProjectProperties(ctx context.Context, projectDir, module string) (string, error) {
	module = strings.TrimSpace(module)
	if module == "" || !strings.HasPrefix(module, ":") || strings.ContainsAny(module, " \t\r\n") {
		return "", fmt.Errorf("module must be a Gradle path such as :app or :feature:login")
	}
	out, err := Gradle(ctx, projectDir, module+":properties")
	return redactSecretProperties(out), err
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
