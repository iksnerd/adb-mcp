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
		{"spaces around equals", `storePassword = "hunter2"`, "storePassword = " + redactedMark},
		{"spaces around equals unquoted", "storePassword = hunter2", "storePassword = " + redactedMark},
		{"quoted value with spaces", `keyPassword="hunter2 s3cret"`, "keyPassword=" + redactedMark},
		{"single quoted with spaces", `apiToken='hunter2 s3cret' next`, "apiToken=" + redactedMark + " next"},
		{"json token", `{"name":"app","token": "hunter2"}`, `{"name":"app","token": ` + redactedMark + `}`},
		{"json hyphen key", `{"api-key":"hunter2","x":1}`, `{"api-key":` + redactedMark + `,"x":1}`},
		{"basic auth", "Authorization: Basic aHVudGVyMjpzM2NyZXQ=", "Authorization: Basic " + redactedMark},
		{"api key header", "X-Api-Key: hunter2", "X-Api-Key: " + redactedMark},
		{"hyphenated auth token header", "x-auth-token: hunter2", "x-auth-token: " + redactedMark},
		{"hyphenated key assignment", "x-api-key=hunter2", "x-api-key=" + redactedMark},
		{"pem block", "before\n-----BEGIN RSA PRIVATE KEY-----\nhunter2abc\ns3cretdef\n-----END RSA PRIVATE KEY-----\nafter", "before\n-----BEGIN PRIVATE KEY----- " + redactedMark + " -----END PRIVATE KEY-----\nafter"},
		{"pem unterminated", "-----BEGIN PRIVATE KEY-----\nhunter2abc\ns3cretdef", "-----BEGIN PRIVATE KEY----- " + redactedMark + " -----END PRIVATE KEY-----"},
		{"task line kept", "> Task :app:compileDebugKotlin UP-TO-DATE", "> Task :app:compileDebugKotlin UP-TO-DATE"},
		{"keystore path kept", "Signing with keystore app.jks", "Signing with keystore app.jks"},
		{"json plain kept", `{"name": "app", "version": "1"}`, `{"name": "app", "version": "1"}`},
		{"header kept", "Content-Type: application/json", "Content-Type: application/json"},
		{"authorization prose kept", "Authorization failed for task", "Authorization failed for task"},
		{"equals across lines kept", "password =\nnext line", "password =\nnext line"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RedactSecrets(tc.in)
			if got != tc.want {
				t.Errorf("RedactSecrets(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if strings.Contains(got, "hunter2") || strings.Contains(got, "s3cret") || strings.Contains(got, "aHVudGVy") {
				t.Errorf("secret leaked: %q", got)
			}
		})
	}
}

func TestRedactSecretPropertiesHyphenKeys(t *testing.T) {
	got := redactSecretProperties("api-key: hunter2\nversion: 1")
	if strings.Contains(got, "hunter2") || !strings.Contains(got, "version: 1") {
		t.Errorf("got %q", got)
	}
}
