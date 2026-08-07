package gradle

import (
	"context"
	"reflect"
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
