package gradle

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestParseProjects(t *testing.T) {
	// Representative `gradlew projects` output for a multi-module build.
	out := `
> Task :projects

------------------------------------------------------------
Root project 'MyApp'
------------------------------------------------------------

Root project 'MyApp'
+--- Project ':app'
+--- Project ':core'
\--- Project ':feature'
     \--- Project ':feature:login'

To see a list of the tasks of a project, run gradlew <project-path>:tasks
`
	got := ParseProjects(out)
	want := []string{":app", ":core", ":feature", ":feature:login"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseProjects = %v, want %v", got, want)
	}

	// A single-module build lists no sub-projects.
	if got := ParseProjects("Root project 'Solo'\nNo sub-projects\n"); got != nil {
		t.Errorf("single-module ParseProjects = %v, want nil", got)
	}
}

func TestListProjectPropertiesRejectsInvalidModule(t *testing.T) {
	if _, err := ListProjectProperties(context.Background(), t.TempDir(), "app"); err == nil {
		t.Fatal("expected a Gradle module path to start with ':'")
	}
}

func TestRedactSecretProperties(t *testing.T) {
	in := `------------------------------------------------------------
Project ':app'
------------------------------------------------------------

NEXUS_USER: svc-example
NEXUS_PASS: hunter2
apiKey: sk-abc123
compileSdkVersion: 34
buildDir: /repo/app/build
`
	got := redactSecretProperties(in)

	for _, secret := range []string{"hunter2", "sk-abc123"} {
		if strings.Contains(got, secret) {
			t.Errorf("redactSecretProperties leaked %q:\n%s", secret, got)
		}
	}
	for _, keep := range []string{"svc-example", "compileSdkVersion: 34", "buildDir: /repo/app/build"} {
		if !strings.Contains(got, keep) {
			t.Errorf("redactSecretProperties dropped non-secret line %q:\n%s", keep, got)
		}
	}
	if !strings.Contains(got, "NEXUS_PASS: ***redacted***") {
		t.Errorf("expected NEXUS_PASS to be redacted:\n%s", got)
	}
	if !strings.Contains(got, "apiKey: ***redacted***") {
		t.Errorf("expected apiKey to be redacted:\n%s", got)
	}
}
