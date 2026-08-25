package uiauto

import (
	"sort"
	"strings"
)

// SuggestLabels names what IS on screen when a find missed, closest candidates
// first. A miss is the moment a caller has full attention and a concrete goal,
// so the error is the highest-conversion place to put this: "no element
// matching X" alone reads as "the element is absent", which sends people
// debugging the app instead of their selector.
//
// byID selects resource ids instead of visible labels, matching the two
// find paths. Ranking is deliberately crude — a shared word or a containment
// either way beats nothing — and ties keep hierarchy order so the top of the
// screen comes first.
func SuggestLabels(elems []Element, query string, byID bool, max int) []string {
	q := Normalize(query)
	if q == "" || max <= 0 {
		return nil
	}
	queryWords := significantWords(q)

	type candidate struct {
		label string
		score int
		order int
	}
	seen := make(map[string]bool)
	var cands []candidate
	for i := range elems {
		label := labelOf(&elems[i], byID)
		if label == "" {
			continue
		}
		norm := Normalize(label)
		if norm == "" || seen[norm] {
			continue
		}
		seen[norm] = true
		cands = append(cands, candidate{label: label, score: similarity(norm, q, queryWords), order: len(cands)})
	}
	sort.SliceStable(cands, func(a, b int) bool { return cands[a].score > cands[b].score })

	var out []string
	for _, c := range cands {
		if len(out) == max {
			break
		}
		out = append(out, c.label)
	}
	return out
}

func labelOf(e *Element, byID bool) string {
	if byID {
		return strings.TrimSpace(e.ResourceID)
	}
	if t := strings.TrimSpace(e.Text); t != "" {
		return t
	}
	return strings.TrimSpace(e.Desc)
}

// similarity scores a normalized candidate against a normalized query: a
// containment either way is the strongest signal, a shared significant word the
// next, everything else zero (still listed, just last).
func similarity(candidate, query string, queryWords []string) int {
	switch {
	case strings.Contains(candidate, query), strings.Contains(query, candidate):
		return 2
	}
	for _, w := range significantWords(candidate) {
		for _, qw := range queryWords {
			if w == qw {
				return 1
			}
		}
	}
	return 0
}

// significantWords splits on whitespace and drops one- and two-letter tokens,
// which match too much to mean anything.
func significantWords(s string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '/' || r == ':' || r == '_' || r == '-' || r == '.'
	}) {
		if len(w) > 2 {
			out = append(out, w)
		}
	}
	return out
}
