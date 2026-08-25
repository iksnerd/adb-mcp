package uiauto

import "testing"

func TestNormalizeFoldsTypographicPunctuation(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"curly apostrophe", "Don’t allow", "don't allow"},
		{"left single quote", "‘quoted’", "'quoted'"},
		{"curly double quotes", "“quoted”", `"quoted"`},
		{"en dash", "Sign–in", "sign-in"},
		{"em dash", "Sign—in", "sign-in"},
		{"minus sign", "−5", "-5"},
		{"ellipsis", "Loading…", "loading..."},
		{"non-breaking space", "Allow once", "allow once"},
		{"narrow no-break space", "12 %", "12 %"},
		{"zero-width space dropped", "Ac​cept", "accept"},
		{"already ascii", "Don't allow", "don't allow"},
		{"trims and lowercases", "  ALLOW  ", "allow"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Normalize(tc.in); got != tc.want {
				t.Errorf("Normalize(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// The field report: tap_on_text("Don't allow") returned "no element matching
// ... found on screen" against the runtime permission dialog, whose label is
// "Don’t allow" with U+2019. describe_ui had the label right all along; only
// the comparison was brittle.
func TestFindByTextMatchesAcrossTypographicApostrophe(t *testing.T) {
	elems := []Element{
		{Text: "Allow", Clickable: true},
		{Text: "Don’t allow", Clickable: true},
	}
	for _, partial := range []bool{true, false} {
		e, ok := FindByText(elems, "Don't allow", partial)
		if !ok {
			t.Fatalf("partial=%t: typed ASCII apostrophe did not match the rendered U+2019 label", partial)
		}
		if e.Text != "Don’t allow" {
			t.Errorf("partial=%t: matched %q, want the curly-quote label", partial, e.Text)
		}
	}
}

func TestFindByTextExactStaysExact(t *testing.T) {
	elems := []Element{{Text: "Allow all the time"}}
	if _, ok := FindByText(elems, "Allow", false); ok {
		t.Error("exact match should not match a longer label; the punctuation fold must not widen matching")
	}
}

func TestFilterByQueryFoldsPunctuation(t *testing.T) {
	elems := []Element{
		{Text: "Don’t allow"},
		{Text: "Allow"},
	}
	if got := FilterByQuery(elems, "don't"); len(got) != 1 || got[0].Text != "Don’t allow" {
		t.Errorf("FilterByQuery folded query = %v, want the curly-quote element", got)
	}
}

func TestDiffersOnlyByPunctuation(t *testing.T) {
	if !DiffersOnlyByPunctuation("Don't allow", "Don’t allow") {
		t.Error("apostrophe variants should differ only by punctuation")
	}
	if DiffersOnlyByPunctuation("Allow", "allow") {
		t.Error("a pure case difference is not a punctuation difference")
	}
	if DiffersOnlyByPunctuation("Allow", "Deny") {
		t.Error("genuinely different labels must not be reported as punctuation-equal")
	}
}
