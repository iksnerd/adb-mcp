package uiauto

import (
	"os"
	"path/filepath"
	"testing"
)

// readTestdata reads a fixture from testdata/. A t.Fatalf (rather than a
// package-level panicking var) keeps a missing fixture localized to the tests
// that need it, with a proper file:line report.
func readTestdata(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(b)
}

func TestParseHierarchy(t *testing.T) {
	elems, err := ParseHierarchy(readTestdata(t, "sample_ui.xml"))
	if err != nil {
		t.Fatalf("ParseHierarchy: %v", err)
	}
	// The pure-layout LinearLayout (no text/desc/id, not clickable) must be dropped.
	if len(elems) != 4 {
		t.Fatalf("expected 4 interesting elements, got %d: %+v", len(elems), elems)
	}

	// The clickable "Continue" ViewGroup: center of [100,2000][980,2120].
	cont := elems[0]
	if cont.Desc != "Continue" || !cont.Clickable {
		t.Errorf("elem0 = %+v, want clickable content-desc Continue", cont)
	}
	if cont.Center.X != 540 || cont.Center.Y != 2060 {
		t.Errorf("Continue center = (%d,%d), want (540,2060)", cont.Center.X, cont.Center.Y)
	}

	// The focused EditText.
	var field *Element
	for i := range elems {
		if elems[i].ResourceID == "com.example:id/field" {
			field = &elems[i]
		}
	}
	if field == nil || !field.Focused || !field.Clickable {
		t.Errorf("expected a focused clickable EditText, got %+v", field)
	}
}

// testdata/wrapper_ui.xml models Material's navigation_bar_item_* pattern:
// deep chains of id-carrying, non-clickable containers, several with bounds
// identical to their parent's.

func TestParseHierarchyFilteredAuto(t *testing.T) {
	elems, hidden, err := ParseHierarchyFiltered(readTestdata(t, "wrapper_ui.xml"), FilterAuto)
	if err != nil {
		t.Fatalf("ParseHierarchyFiltered: %v", err)
	}
	// Dropped in auto: the bare FrameLayout (no label/id/click) and the
	// active_indicator_view (label-less, non-clickable, bounds identical to its
	// parent icon_container). Kept: content_container, icon_container and
	// icon_view (ids + distinct bounds), the label, and the clickable tab.
	if len(elems) != 5 {
		t.Fatalf("auto: expected 5 elements, got %d: %+v", len(elems), elems)
	}
	if hidden != 2 {
		t.Errorf("auto: expected 2 hidden nodes, got %d", hidden)
	}
	for _, e := range elems {
		if e.ResourceID == "app:id/navigation_bar_item_active_indicator_view" {
			t.Errorf("identical-bounds wrapper should have been dropped: %+v", e)
		}
	}
}

func TestParseHierarchyFilteredAll(t *testing.T) {
	elems, hidden, err := ParseHierarchyFiltered(readTestdata(t, "wrapper_ui.xml"), FilterAll)
	if err != nil {
		t.Fatalf("ParseHierarchyFiltered: %v", err)
	}
	if len(elems) != 7 || hidden != 0 {
		t.Fatalf("all: expected 7 elements / 0 hidden, got %d / %d", len(elems), hidden)
	}
}

func TestParseHierarchyFilteredClickable(t *testing.T) {
	elems, hidden, err := ParseHierarchyFiltered(readTestdata(t, "wrapper_ui.xml"), FilterClickable)
	if err != nil {
		t.Fatalf("ParseHierarchyFiltered: %v", err)
	}
	if len(elems) != 1 || !elems[0].Clickable {
		t.Fatalf("clickable: expected exactly the clickable tab, got %+v", elems)
	}
	if hidden != 6 {
		t.Errorf("clickable: expected 6 hidden nodes, got %d", hidden)
	}
}

func TestParseUIFilter(t *testing.T) {
	for in, want := range map[string]UIFilter{"": FilterAuto, "auto": FilterAuto, "ALL": FilterAll, " clickable ": FilterClickable} {
		got, err := ParseUIFilter(in)
		if err != nil || got != want {
			t.Errorf("ParseUIFilter(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	if _, err := ParseUIFilter("everything"); err == nil {
		t.Error("expected an error for an unknown filter name")
	}
}

func TestFilterByQuery(t *testing.T) {
	elems, _ := ParseHierarchy(readTestdata(t, "sample_ui.xml"))
	if got := FilterByQuery(elems, "continue"); len(got) != 2 { // ViewGroup desc + TextView text
		t.Errorf("query 'continue': got %d elements, want 2: %+v", len(got), got)
	}
	if got := FilterByQuery(elems, "com.example:id/field"); len(got) != 1 {
		t.Errorf("query by resource id: got %d elements, want 1", len(got))
	}
	if got := FilterByQuery(elems, "nope"); len(got) != 0 {
		t.Errorf("query 'nope': got %d elements, want 0", len(got))
	}
	if got := FilterByQuery(elems, "  "); len(got) != len(elems) {
		t.Errorf("blank query should keep everything")
	}
}

func TestParseBounds(t *testing.T) {
	b, ok := parseBounds("[12,34][560,780]")
	if !ok || b.X1 != 12 || b.Y1 != 34 || b.X2 != 560 || b.Y2 != 780 {
		t.Errorf("parseBounds = %+v, ok=%v", b, ok)
	}
	if _, ok := parseBounds("garbage"); ok {
		t.Errorf("expected parse failure on garbage")
	}
}

func TestFindByText(t *testing.T) {
	elems, _ := ParseHierarchy(readTestdata(t, "sample_ui.xml"))

	// Exact match on the visible label should prefer the clickable ViewGroup
	// (which carries the same text as its content-desc) — its center is tappable.
	e, ok := FindByText(elems, "Continue", false)
	if !ok {
		t.Fatal("Continue not found")
	}
	if !e.Clickable {
		t.Errorf("FindByText returned non-clickable element: %+v", e)
	}

	// Partial, case-insensitive.
	if _, ok := FindByText(elems, "enter your", true); !ok {
		t.Error("expected partial match on 'enter your'")
	}
	// Non-existent.
	if _, ok := FindByText(elems, "nope", true); ok {
		t.Error("did not expect a match for 'nope'")
	}
	// An empty/whitespace query must never match (a "" substring would match
	// every element).
	if _, ok := FindByText(elems, "  ", true); ok {
		t.Error("blank query must not match anything")
	}
}

func TestFindByResourceID(t *testing.T) {
	elems, _ := ParseHierarchy(readTestdata(t, "sample_ui.xml"))

	e, ok := FindByResourceID(elems, "com.example:id/field", false)
	if !ok || e.ResourceID != "com.example:id/field" {
		t.Fatalf("exact match failed: %+v, ok=%v", e, ok)
	}

	// Partial, case-insensitive match on a bare id suffix.
	if _, ok := FindByResourceID(elems, "FIELD", true); !ok {
		t.Error("expected partial match on 'FIELD'")
	}

	// Non-existent.
	if _, ok := FindByResourceID(elems, "nope", true); ok {
		t.Error("did not expect a match for 'nope'")
	}
	// An empty/whitespace query must never match (a "" substring would match
	// every id-carrying element and tap an arbitrary one).
	if _, ok := FindByResourceID(elems, "", true); ok {
		t.Error("blank query must not match anything")
	}

	// Among several id-carrying matches, prefer the clickable one.
	wrapperElems, _ := ParseHierarchy(readTestdata(t, "wrapper_ui.xml"))
	tab, ok := FindByResourceID(wrapperElems, "tab_settings", true)
	if !ok || !tab.Clickable {
		t.Errorf("expected the clickable tab_settings match, got %+v ok=%v", tab, ok)
	}
}

func TestElementAt(t *testing.T) {
	container := Element{ResourceID: "root", Bounds: Bounds{X1: 0, Y1: 0, X2: 1000, Y2: 2000}}
	button := Element{ResourceID: "btn", Clickable: true, Bounds: Bounds{X1: 100, Y1: 100, X2: 300, Y2: 200}}
	// A non-clickable wrapper with the SAME bounds as the button — the tie must
	// break toward the clickable one.
	wrapper := Element{ResourceID: "btn_wrapper", Bounds: Bounds{X1: 100, Y1: 100, X2: 300, Y2: 200}}
	elems := []Element{container, wrapper, button}

	// Inside the button: the smallest containing element wins (not the root).
	if e, ok := ElementAt(elems, 200, 150); !ok || e.ResourceID != "btn" {
		t.Errorf("hit inside button = %+v ok=%v, want btn (clickable tie-break over wrapper)", e, ok)
	}
	// Inside the container but outside the button: falls back to the container.
	if e, ok := ElementAt(elems, 500, 1000); !ok || e.ResourceID != "root" {
		t.Errorf("hit in container = %+v ok=%v, want root", e, ok)
	}
	// Off every element's bounds: no hit.
	if _, ok := ElementAt(elems, 5000, 5000); ok {
		t.Error("expected no hit for an off-screen coordinate")
	}
	// Edge inclusive: the bottom-right corner still counts as inside.
	if _, ok := ElementAt(elems, 1000, 2000); !ok {
		t.Error("expected the bottom-right boundary pixel to count as a hit")
	}
}
