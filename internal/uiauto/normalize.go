package uiauto

import "strings"

// typographic folds the punctuation Android renders onto the punctuation a
// caller can actually type. System dialogs use curly quotes ("Don’t allow" is
// U+2019, not an ASCII apostrophe), en/em dashes, ellipsis characters and
// non-breaking spaces; a hand-written selector almost never does. Before this
// fold, tap_on_text("Don't allow") missed the runtime permission dialog and
// reported it as absent — the data was right, only the comparison was brittle.
//
// The zero-width characters are dropped rather than mapped: they carry no
// visual width, so a caller has no way to know they are there.
var typographic = strings.NewReplacer(
	"‘", "'", // ‘ left single quote
	"’", "'", // ’ right single quote / apostrophe
	"‚", "'", // ‚ single low quote
	"‛", "'", // ‛ single high-reversed quote
	"ʼ", "'", // ʼ modifier letter apostrophe
	"′", "'", // ′ prime
	"“", `"`, // “ left double quote
	"”", `"`, // ” right double quote
	"„", `"`, // „ double low quote
	"‟", `"`, // ‟ double high-reversed quote
	"″", `"`, // ″ double prime
	"‐", "-", // ‐ hyphen
	"‑", "-", // ‑ non-breaking hyphen
	"‒", "-", // ‒ figure dash
	"–", "-", // – en dash
	"—", "-", // — em dash
	"―", "-", // ― horizontal bar
	"−", "-", // − minus sign
	"…", "...", // … ellipsis
	" ", " ", // no-break space
	" ", " ", // figure space
	" ", " ", // thin space
	" ", " ", // hair space
	" ", " ", // narrow no-break space
	"　", " ", // ideographic space
	"\u200b", "", // zero-width space
	"\u200c", "", // zero-width non-joiner
	"\u200d", "", // zero-width joiner
	"\ufeff", "", // zero-width no-break space / BOM
)

// Normalize prepares a label or a caller's query for comparison: typographic
// punctuation folded to ASCII, case flattened, surrounding whitespace trimmed.
// Both sides of every text match go through it, so the fold can never make a
// match *narrower* than a raw comparison would.
func Normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(typographic.Replace(s)))
}

// DiffersOnlyByPunctuation reports whether two strings match once typographic
// punctuation is folded but not before — i.e. the exact case this fold exists
// to absorb. Callers use it to explain a match that a literal comparison would
// have missed.
func DiffersOnlyByPunctuation(a, b string) bool {
	raw := strings.ToLower(strings.TrimSpace(a)) == strings.ToLower(strings.TrimSpace(b))
	return !raw && Normalize(a) == Normalize(b)
}
