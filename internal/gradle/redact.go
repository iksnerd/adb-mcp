package gradle

import (
	"regexp"
	"strings"
)

const redactedMark = "***redacted***"

// pemPlaceholder stands in for a block while later markers are searched.
const pemPlaceholder = "\x00PEM\x00"

const pemRedacted = "-----BEGIN PRIVATE KEY----- " + redactedMark + " -----END PRIVATE KEY-----"

var (
	// assignedSecretRe matches `name=value` where name looks secret-shaped:
	// -PmavenToken=…, systemProp.repoPassword=…, an --info dump of
	// gradle.properties. Reuses the key heuristic of the properties task.
	// Spaces or tabs may surround the "=", and the value may be quoted with
	// spaces inside; newlines never bridge name and value.
	assignedSecretRe = regexp.MustCompile(`(?i)([\w.\-]*(?:pass|secret|token|key|credential)[\w.\-]*[ \t]*=[ \t]*)("[^"\n]*"|'[^'\n]*'|\S+)`)
	// jsonSecretRe matches a JSON member whose quoted key looks secret-shaped:
	// "token": "x", "api-key":"x".
	jsonSecretRe = regexp.MustCompile(`(?i)("[\w.\-]*(?:pass|secret|token|key|credential)[\w.\-]*"[ \t]*:[ \t]*)("(?:[^"\\\n]|\\.)*"|[^\s,}\]]+)`)
	// authSchemeRe matches non-bearer Authorization credentials (Basic, ...).
	authSchemeRe = regexp.MustCompile(`(?i)(\bauthorization[ \t]*:[ \t]*(?:basic|digest|negotiate|token)[ \t]+)\S+`)
	// secretHeaderRe matches hyphenated secret headers such as X-Api-Key,
	// X-Auth-Token and Api-Key, in "Name: value" form.
	secretHeaderRe = regexp.MustCompile(`(?i)(\b(?:x-[\w\-]*(?:key|token|secret|password|auth)[\w\-]*|[\w\-]*(?:api-key|auth-token|access-token|secret-key|private-key))[ \t]*:[ \t]*)[^\r\n]+`)
	// pemBlockRe matches a whole PEM private key block; pemBeginRe/pemEndRe
	// are the markers redactingWriter tracks across lines.
	pemBlockRe = regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`)
	pemBeginRe = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)
	pemEndRe   = regexp.MustCompile(`-----END [A-Z ]*PRIVATE KEY-----`)
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
	// PEM first: a key body is not line-shaped, and a begin marker with no
	// end (truncated output) drops everything after it rather than risk a leak.
	output = pemBlockRe.ReplaceAllString(output, pemPlaceholder)
	if loc := pemBeginRe.FindStringIndex(output); loc != nil {
		output = output[:loc[0]] + pemPlaceholder
	}
	output = strings.ReplaceAll(output, pemPlaceholder, pemRedacted)
	output = redactSecretProperties(output)
	output = assignedSecretRe.ReplaceAllString(output, "${1}"+redactedMark)
	output = jsonSecretRe.ReplaceAllString(output, "${1}"+redactedMark)
	output = bearerRe.ReplaceAllString(output, "${1}"+redactedMark)
	output = authSchemeRe.ReplaceAllString(output, "${1}"+redactedMark)
	output = secretHeaderRe.ReplaceAllString(output, "${1}"+redactedMark)
	return urlCredentialsRe.ReplaceAllString(output, "${1}"+redactedMark+"@")
}
