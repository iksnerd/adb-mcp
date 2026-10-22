package gradle

import "regexp"

const redactedMark = "***redacted***"

var (
	// assignedSecretRe matches `name=value` where name looks secret-shaped:
	// -PmavenToken=…, systemProp.repoPassword=…, an --info dump of
	// gradle.properties. Reuses the key heuristic of the properties task.
	assignedSecretRe = regexp.MustCompile(`(?i)([\w.\-]*(?:pass|secret|token|key|credential)[\w.\-]*)=(\S+)`)
	// bearerRe matches an Authorization-style bearer token.
	bearerRe = regexp.MustCompile(`(?i)(bearer\s+)\S+`)
	// urlCredentialsRe matches the password in https://user:password@host.
	urlCredentialsRe = regexp.MustCompile(`(://[^/\s:@]+:)[^@\s/]+@`)
)

// RedactSecrets masks credential-shaped values in Gradle output before it is
// returned to an agent. Gradle with --info or --debug echoes properties from
// ~/.gradle/gradle.properties (private Maven repo auth) and repository URLs,
// none of which should land in a transcript. Pure string work.
func RedactSecrets(output string) string {
	output = redactSecretProperties(output)
	output = assignedSecretRe.ReplaceAllString(output, "${1}="+redactedMark)
	output = bearerRe.ReplaceAllString(output, "${1}"+redactedMark)
	return urlCredentialsRe.ReplaceAllString(output, "${1}"+redactedMark+"@")
}
