package android

import "testing"

const sampleUI = `<?xml version='1.0' encoding='UTF-8' standalone='yes' ?>
<hierarchy rotation="0">
  <node index="0" text="" class="android.widget.FrameLayout" bounds="[0,0][1080,2400]">
    <node index="0" text="" content-desc="Continue" class="android.view.ViewGroup" clickable="true" bounds="[100,2000][980,2120]">
      <node index="0" text="Continue" class="android.widget.TextView" clickable="false" bounds="[400,2040][680,2080]"/>
    </node>
    <node index="1" text="Enter your PIN" class="android.widget.TextView" clickable="false" bounds="[300,400][780,460]"/>
    <node index="2" text="" content-desc="" class="android.widget.LinearLayout" clickable="false" bounds="[0,500][1080,600]"/>
    <node index="3" resource-id="com.example:id/field" text="" class="android.widget.EditText" focused="true" clickable="true" bounds="[100,700][980,780]"/>
  </node>
</hierarchy>`

func TestParseHierarchy(t *testing.T) {
	elems, err := ParseHierarchy(sampleUI)
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

// wrapperUI models Material's navigation_bar_item_* pattern: deep chains of
// id-carrying, non-clickable containers, several with bounds identical to
// their parent's.
const wrapperUI = `<?xml version='1.0' encoding='UTF-8' standalone='yes' ?>
<hierarchy rotation="0">
  <node text="" class="android.widget.FrameLayout" bounds="[0,0][1080,2400]">
    <node text="" resource-id="app:id/navigation_bar_item_content_container" clickable="false" bounds="[221,2182][416,2361]">
      <node text="" resource-id="app:id/navigation_bar_item_icon_container" clickable="false" bounds="[234,2182][402,2266]">
        <node text="" resource-id="app:id/navigation_bar_item_active_indicator_view" clickable="false" bounds="[234,2182][402,2266]"/>
        <node text="" resource-id="app:id/navigation_bar_item_icon_view" clickable="false" bounds="[286,2192][349,2255]"/>
      </node>
      <node text="Settings" resource-id="app:id/navigation_bar_item_large_label_view" clickable="false" bounds="[221,2277][416,2361]"/>
    </node>
    <node text="" resource-id="app:id/tab_settings" clickable="true" bounds="[221,2182][416,2361]"/>
  </node>
</hierarchy>`

func TestParseHierarchyFilteredAuto(t *testing.T) {
	elems, hidden, err := ParseHierarchyFiltered(wrapperUI, FilterAuto)
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
	elems, hidden, err := ParseHierarchyFiltered(wrapperUI, FilterAll)
	if err != nil {
		t.Fatalf("ParseHierarchyFiltered: %v", err)
	}
	if len(elems) != 7 || hidden != 0 {
		t.Fatalf("all: expected 7 elements / 0 hidden, got %d / %d", len(elems), hidden)
	}
}

func TestParseHierarchyFilteredClickable(t *testing.T) {
	elems, hidden, err := ParseHierarchyFiltered(wrapperUI, FilterClickable)
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
	elems, _ := ParseHierarchy(sampleUI)
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

func TestDialpadPoint(t *testing.T) {
	// A 300x400 pad at origin: colW=100, rowH=100, so centers land at 50/150/250
	// horizontally and 50/150/250/350 vertically.
	b := Bounds{X1: 0, Y1: 0, X2: 300, Y2: 400}
	cases := map[rune]Point{
		'1': {50, 50}, '2': {150, 50}, '3': {250, 50},
		'4': {50, 150}, '5': {150, 150}, '6': {250, 150},
		'7': {50, 250}, '8': {150, 250}, '9': {250, 250},
		'0': {150, 350},
	}
	for d, want := range cases {
		got, ok := dialpadPoint(b, d)
		if !ok || got != want {
			t.Errorf("dialpadPoint(%q) = %+v ok=%v, want %+v", string(d), got, ok, want)
		}
	}
	if _, ok := dialpadPoint(b, 'x'); ok {
		t.Errorf("expected non-digit to fail")
	}
}

func TestDialpadPointOffset(t *testing.T) {
	// Pad not at origin: [100,200]-[400,600]. colW=100, rowH=100.
	b := Bounds{X1: 100, Y1: 200, X2: 400, Y2: 600}
	got, _ := dialpadPoint(b, '5')
	if want := (Point{X: 250, Y: 350}); got != want {
		t.Errorf("dialpadPoint('5') = %+v, want %+v", got, want)
	}
	got, _ = dialpadPoint(b, '0')
	if want := (Point{X: 250, Y: 550}); got != want {
		t.Errorf("dialpadPoint('0') = %+v, want %+v", got, want)
	}
}

func TestFindByText(t *testing.T) {
	elems, _ := ParseHierarchy(sampleUI)

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
}
