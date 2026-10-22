package gradle

import (
	"strings"
	"testing"
)

func TestRedactSecrets(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"properties style", "NEXUS_PASS: hunter2", "NEXUS_PASS: ***redacted***"},
		{"key=value", "systemProp.repoPassword=hunter2", "systemProp.repoPassword=***redacted***"},
		{"gradle flag", "> Task :app:x -PmavenToken=abc123 --info", "> Task :app:x -PmavenToken=***redacted*** --info"},
		{"bearer header", "Authorization: Bearer eyJhbGciOi.abc", "Authorization: Bearer ***redacted***"},
		{"url credentials", "Download https://deploy:s3cret@repo.example.com/m.pom", "Download https://deploy:***redacted***@repo.example.com/m.pom"},
		{"harmless line kept", "compileSdkVersion: 35", "compileSdkVersion: 35"},
		{"harmless equals kept", "org.gradle.jvmargs=-Xmx2g", "org.gradle.jvmargs=-Xmx2g"},
		{"build output kept", "BUILD SUCCESSFUL in 4m 12s", "BUILD SUCCESSFUL in 4m 12s"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RedactSecrets(tc.in)
			if got != tc.want {
				t.Errorf("RedactSecrets(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if strings.Contains(got, "hunter2") || strings.Contains(got, "s3cret") {
				t.Errorf("secret leaked: %q", got)
			}
		})
	}
}
