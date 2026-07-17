package uiauto

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Element is a flattened, model-friendly view of a uiautomator node. Bounds and
// Center are in true device pixels, so Center can be passed straight to Tap.
type Element struct {
	Text       string `json:"text,omitempty"`
	Desc       string `json:"content_desc,omitempty"`
	ResourceID string `json:"resource_id,omitempty"`
	Class      string `json:"class,omitempty"`
	Clickable  bool   `json:"clickable"`
	Focused    bool   `json:"focused,omitempty"`
	Bounds     Bounds `json:"bounds"`
	Center     Point  `json:"center"`
}

// Bounds is the pixel rectangle of an element: [X1,Y1] top-left, [X2,Y2] bottom-right.
type Bounds struct {
	X1 int `json:"x1"`
	Y1 int `json:"y1"`
	X2 int `json:"x2"`
	Y2 int `json:"y2"`
}

// Point is a tap coordinate in true device pixels.
type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// xmlNode mirrors the <node> element emitted by `uiautomator dump`.
type xmlNode struct {
	Text      string    `xml:"text,attr"`
	Desc      string    `xml:"content-desc,attr"`
	Resource  string    `xml:"resource-id,attr"`
	Class     string    `xml:"class,attr"`
	Clickable string    `xml:"clickable,attr"`
	Focused   string    `xml:"focused,attr"`
	Bounds    string    `xml:"bounds,attr"`
	Children  []xmlNode `xml:"node"`
}

type xmlHierarchy struct {
	XMLName xml.Name  `xml:"hierarchy"`
	Nodes   []xmlNode `xml:"node"`
}

var boundsRe = regexp.MustCompile(`\[(-?\d+),(-?\d+)\]\[(-?\d+),(-?\d+)\]`)

// UIFilter selects how aggressively ParseHierarchyFiltered prunes the tree.
type UIFilter string

const (
	// FilterAuto keeps nodes carrying text, a content-description, a resource
	// id, or a clickable flag — and additionally drops textless, descless,
	// non-clickable nodes whose bounds are identical to their parent's (pure
	// wrappers that add no spatial information, e.g. Material's 5-deep
	// navigation_bar_item_* chains).
	FilterAuto UIFilter = "auto"
	// FilterAll keeps every node with parseable bounds. Absence from this view
	// is trustworthy: the element is genuinely not in the hierarchy.
	FilterAll UIFilter = "all"
	// FilterClickable keeps only clickable nodes — the smallest useful view
	// when all you need is tap targets.
	FilterClickable UIFilter = "clickable"
)

// ParseUIFilter validates a user-supplied filter name; empty means FilterAuto.
func ParseUIFilter(s string) (UIFilter, error) {
	switch UIFilter(strings.ToLower(strings.TrimSpace(s))) {
	case "", FilterAuto:
		return FilterAuto, nil
	case FilterAll:
		return FilterAll, nil
	case FilterClickable:
		return FilterClickable, nil
	default:
		return "", fmt.Errorf("filter must be auto, all, or clickable, got %q", s)
	}
}

// ParseHierarchy parses uiautomator XML into a flat list of interesting
// elements using the default (auto) filter. Kept for callers that only need
// elements for text lookup (wait_for_text, enter_pin), where hidden layout
// nodes are irrelevant.
func ParseHierarchy(xmlData string) ([]Element, error) {
	elems, _, err := ParseHierarchyFiltered(xmlData, FilterAuto)
	return elems, err
}

// ParseHierarchyFiltered parses uiautomator XML into a flat element list under
// the given filter and also reports how many bounded nodes the filter hid —
// so a caller can distinguish "absent from the hierarchy" from "filtered out".
func ParseHierarchyFiltered(xmlData string, filter UIFilter) (elems []Element, hidden int, err error) {
	var h xmlHierarchy
	if err := xml.Unmarshal([]byte(xmlData), &h); err != nil {
		return nil, 0, fmt.Errorf("parse uiautomator xml: %w", err)
	}
	for i := range h.Nodes {
		walk(&h.Nodes[i], Bounds{X1: -1, Y1: -1, X2: -1, Y2: -1}, filter, &elems, &hidden)
	}
	return elems, hidden, nil
}

func walk(n *xmlNode, parent Bounds, filter UIFilter, out *[]Element, hidden *int) {
	clickable := n.Clickable == "true"
	b, bounded := parseBounds(n.Bounds)
	if bounded {
		if keepNode(n, clickable, b, parent, filter) {
			*out = append(*out, Element{
				Text:       n.Text,
				Desc:       n.Desc,
				ResourceID: n.Resource,
				Class:      n.Class,
				Clickable:  clickable,
				Focused:    n.Focused == "true",
				Bounds:     b,
				Center:     Point{X: (b.X1 + b.X2) / 2, Y: (b.Y1 + b.Y2) / 2},
			})
		} else {
			*hidden++
		}
	} else {
		b = parent // unparseable bounds: children compare against the last known rect
	}
	for i := range n.Children {
		walk(&n.Children[i], b, filter, out, hidden)
	}
}

func keepNode(n *xmlNode, clickable bool, b, parent Bounds, filter UIFilter) bool {
	switch filter {
	case FilterAll:
		return true
	case FilterClickable:
		return clickable
	}
	hasText := strings.TrimSpace(n.Text) != "" || strings.TrimSpace(n.Desc) != ""
	if !hasText && !clickable && b == parent {
		return false // pure wrapper: no label, not tappable, same rect as its parent
	}
	return hasText || strings.TrimSpace(n.Resource) != "" || clickable
}

func parseBounds(s string) (Bounds, bool) {
	m := boundsRe.FindStringSubmatch(s)
	if m == nil {
		return Bounds{}, false
	}
	x1, _ := strconv.Atoi(m[1])
	y1, _ := strconv.Atoi(m[2])
	x2, _ := strconv.Atoi(m[3])
	y2, _ := strconv.Atoi(m[4])
	return Bounds{X1: x1, Y1: y1, X2: x2, Y2: y2}, true
}

// FilterByQuery keeps elements whose text, content-description, or resource id
// contains query (case-insensitive). It answers "is X on this screen?" without
// returning the whole tree.
func FilterByQuery(elems []Element, query string) []Element {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return elems
	}
	var out []Element
	for i := range elems {
		e := &elems[i]
		if strings.Contains(strings.ToLower(e.Text), q) ||
			strings.Contains(strings.ToLower(e.Desc), q) ||
			strings.Contains(strings.ToLower(e.ResourceID), q) {
			out = append(out, *e)
		}
	}
	return out
}

// FindByText returns the first element whose text or content-description matches
// query. When partial is true it does a case-insensitive substring match;
// otherwise it requires a case-insensitive exact match. Clickable elements are
// preferred over non-clickable ones when several match.
func FindByText(elems []Element, query string, partial bool) (Element, bool) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return Element{}, false // an empty query would substring-match everything
	}
	return findFirst(elems, func(e *Element) bool {
		return matches(e.Text, q, partial) || matches(e.Desc, q, partial)
	})
}

// FindByResourceID returns the first element whose resource id matches query.
// When partial is true it does a case-insensitive substring match (so a bare
// "submit_button" matches "com.example.app:id/submit_button"); otherwise it
// requires a case-insensitive exact match. Clickable elements are preferred
// over non-clickable ones when several match.
func FindByResourceID(elems []Element, query string, partial bool) (Element, bool) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return Element{}, false // an empty query would substring-match everything
	}
	return findFirst(elems, func(e *Element) bool {
		return matches(e.ResourceID, q, partial)
	})
}

// findFirst returns the first element satisfying pred, preferring a clickable
// match over a non-clickable one — this preference policy is shared by every
// find-and-tap path so text- and id-addressed taps resolve a screen the same way.
func findFirst(elems []Element, pred func(*Element) bool) (Element, bool) {
	fallback := -1
	for i := range elems {
		e := &elems[i]
		if pred(e) {
			if e.Clickable {
				return *e, true
			}
			if fallback < 0 {
				fallback = i
			}
		}
	}
	if fallback >= 0 {
		return elems[fallback], true
	}
	return Element{}, false
}

func matches(field, q string, partial bool) bool {
	f := strings.ToLower(strings.TrimSpace(field))
	if f == "" {
		return false
	}
	if partial {
		return strings.Contains(f, q)
	}
	return f == q
}
