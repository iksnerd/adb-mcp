package gradle

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/iksnerd/adb-mcp/internal/concurrent"
	"github.com/iksnerd/adb-mcp/internal/sdk"
)

// ToolchainReport describes the host-side build tools, for doctor. The device
// layer's checks say nothing about whether a build can run: Gradle needs a JDK,
// and scaffold_android_project needs a system `gradle` to generate a wrapper.
// Both fail late and confusingly when missing — a JDK-less machine reports a
// Gradle startup error, not "install a JDK".
func ToolchainReport(ctx context.Context) string {
	var javaOut, gradleOut string
	var javaErr, gradleErr error
	concurrent.RunAll(
		func() { javaOut, javaErr = probe(ctx, "java", "-version") },
		func() { gradleOut, gradleErr = probe(ctx, "gradle", "--version") },
	)

	var b strings.Builder
	switch {
	case javaErr != nil:
		fmt.Fprintf(&b, "✗ java: not runnable (%v) — Gradle needs a JDK (17+ for current AGP)\n", javaErr)
	default:
		// `java -version` prints to stderr and leads with the version line.
		fmt.Fprintf(&b, "✓ java: %s\n", firstLine(strings.TrimSpace(javaOut)))
	}

	if gradleErr != nil {
		// Only needed to generate a wrapper; a project that already has
		// ./gradlew builds fine without it, so this is a warning, not a failure.
		b.WriteString("⚠ gradle (system): not on PATH — only needed so scaffold_android_project can generate a Gradle wrapper; projects that already have ./gradlew are unaffected\n")
	} else {
		fmt.Fprintf(&b, "✓ gradle (system): %s\n", gradleVersion(gradleOut))
	}
	return strings.TrimRight(b.String(), "\n")
}

func probe(ctx context.Context, name string, args ...string) (string, error) {
	bin, err := exec.LookPath(name)
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = sdk.CommandEnv()
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// gradleVersion pulls the "Gradle X.Y" line out of `gradle --version`, whose
// output is a banner several lines tall rather than a single version string.
func gradleVersion(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); strings.HasPrefix(line, "Gradle ") {
			return line
		}
	}
	return firstLine(strings.TrimSpace(out))
}
