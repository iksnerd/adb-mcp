package uiauto

import (
	"slices"
	"testing"
)

func TestSuggestLabelsRanksNearestFirst(t *testing.T) {
	elems := []Element{
		{Text: "Settings"},
		{Text: "Allow while using the app"},
		{Desc: "Profile picture"},
		{Text: "Don’t allow"},
	}
	got := SuggestLabels(elems, "Allow", false, 5)
	if len(got) != 4 {
		t.Fatalf("got %d suggestions, want all 4 labelled elements: %v", len(got), got)
	}
	// Both "allow" labels contain the query, so they outrank the unrelated ones;
	// hierarchy order breaks the tie between them.
	want := []string{"Allow while using the app", "Don’t allow"}
	if !slices.Equal(got[:2], want) {
		t.Errorf("top suggestions = %v, want %v", got[:2], want)
	}
}

func TestSuggestLabelsRespectsMaxAndDedupes(t *testing.T) {
	elems := []Element{
		{Text: "Save"}, {Text: "Save"}, {Text: "Cancel"}, {Text: "Delete"}, {Text: "Share"},
	}
	got := SuggestLabels(elems, "Submit", false, 2)
	if len(got) != 2 {
		t.Fatalf("got %d suggestions, want 2 (max)", len(got))
	}
	if got[0] == got[1] {
		t.Errorf("duplicate labels were not collapsed: %v", got)
	}
}

func TestSuggestLabelsByResourceID(t *testing.T) {
	elems := []Element{
		{Text: "Submit", ResourceID: "com.example:id/submit_button"},
		{Text: "Cancel", ResourceID: "com.example:id/cancel_button"},
	}
	got := SuggestLabels(elems, "submit_btn", true, 5)
	if len(got) == 0 || got[0] != "com.example:id/submit_button" {
		t.Errorf("id suggestions = %v, want the submit id first", got)
	}
}

func TestSuggestLabelsEmptyInputs(t *testing.T) {
	if got := SuggestLabels(nil, "anything", false, 5); got != nil {
		t.Errorf("no elements should yield no suggestions, got %v", got)
	}
	if got := SuggestLabels([]Element{{Text: "Save"}}, "", false, 5); got != nil {
		t.Errorf("an empty query should yield no suggestions, got %v", got)
	}
}
