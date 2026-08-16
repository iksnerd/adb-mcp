// Command docsync keeps the hand-written counts in the docs agreeing with the
// code: how many tools are registered, how many guide resources are served,
// how many areas the tool reference is split into, and the current version.
//
// VERSION is the single source for the version: this also writes it into
// cmd/adb-mcp/main.go and server.json (both the "version" field and the tag on
// the OCI package), so a release only ever needs VERSION edited by hand.
//
// These drifted repeatedly because nothing checked them. The release gate can
// only cover tracked files, and the roadmap banner lives in a gitignored file
// it can never see, so the same numbers were re-typed by hand every release
// and were wrong more than once.
//
//	go run ./scripts/docsync          # rewrite the counts in place
//	go run ./scripts/docsync -check   # fail if anything is stale (CI)
//
// -check covers only tracked files, so it is safe to run anywhere. The
// gitignored roadmap is updated when present and skipped when absent.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Counts are derived from the source of truth, never from another doc.
type counts struct {
	tools   int
	guides  int
	areas   int
	version string
}

var (
	reRegisteredTool = regexp.MustCompile(`(?m)^\tadd\(s, "([a-z_]+)"`)
	reGuideURI       = regexp.MustCompile(`(?m)^\t\turi:\s+"android://guide/`)
	reToolsArea      = regexp.MustCompile(`(?m)^### `)
	reDocumentedTool = regexp.MustCompile("`([a-z_]+)`")
)

func main() {
	check := flag.Bool("check", false, "verify instead of rewriting; exit 1 if any count is stale")
	root := flag.String("root", ".", "repository root")
	flag.Parse()

	c, err := derive(*root)
	if err != nil {
		fail(err)
	}
	fmt.Printf("derived: %d tools, %d guide resources, %d areas, version %s\n", c.tools, c.guides, c.areas, c.version)

	if problems := undocumented(*root); len(problems) > 0 {
		fail(fmt.Errorf("registered but absent from docs/TOOLS.md: %s", strings.Join(problems, ", ")))
	}

	var stale []string
	for _, f := range rewrites(c) {
		path := filepath.Join(*root, f.path)
		before, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) && f.optional {
				continue // the roadmap is gitignored; absent on a fresh clone
			}
			fail(err)
		}
		after := f.apply(string(before))
		if after == string(before) {
			continue
		}
		if *check {
			stale = append(stale, f.path)
			continue
		}
		if err := os.WriteFile(path, []byte(after), 0o644); err != nil {
			fail(err)
		}
		fmt.Println("updated", f.path)
	}

	if len(stale) > 0 {
		fmt.Fprintf(os.Stderr, "\nstale counts in: %s\nrun `make docs` and commit the result\n", strings.Join(stale, ", "))
		os.Exit(1)
	}
	if *check {
		fmt.Println("all doc counts agree with the code")
	}
}

func derive(root string) (counts, error) {
	var c counts
	register, err := os.ReadFile(filepath.Join(root, "internal/tools/register.go"))
	if err != nil {
		return c, err
	}
	guides, err := os.ReadFile(filepath.Join(root, "internal/guides/guides.go"))
	if err != nil {
		return c, err
	}
	toolsDoc, err := os.ReadFile(filepath.Join(root, "docs/TOOLS.md"))
	if err != nil {
		return c, err
	}
	version, err := os.ReadFile(filepath.Join(root, "VERSION"))
	if err != nil {
		return c, err
	}

	c.tools = len(reRegisteredTool.FindAllSubmatch(register, -1))
	c.guides = len(reGuideURI.FindAll(guides, -1))
	// Areas are the "### " sections of the tool reference — the same headings
	// the README's bullet list mirrors.
	c.areas = len(reToolsArea.FindAll(toolsDoc, -1))
	c.version = strings.TrimSpace(string(version))

	if c.tools == 0 || c.guides == 0 || c.areas == 0 {
		return c, fmt.Errorf("derived a zero count (tools=%d guides=%d areas=%d) — the scan patterns no longer match the source", c.tools, c.guides, c.areas)
	}
	return c, nil
}

// undocumented reports registered tools with no mention in docs/TOOLS.md.
// Counting table rows would not catch this: several tools share a row
// ("`install_app` / `uninstall_app`"), so the totals can agree while a tool is
// missing.
func undocumented(root string) []string {
	register, err := os.ReadFile(filepath.Join(root, "internal/tools/register.go"))
	if err != nil {
		fail(err)
	}
	doc, err := os.ReadFile(filepath.Join(root, "docs/TOOLS.md"))
	if err != nil {
		fail(err)
	}
	documented := map[string]bool{}
	for _, m := range reDocumentedTool.FindAllSubmatch(doc, -1) {
		documented[string(m[1])] = true
	}
	var missing []string
	for _, m := range reRegisteredTool.FindAllSubmatch(register, -1) {
		if name := string(m[1]); !documented[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return missing
}

type rewrite struct {
	path     string
	optional bool
	subs     []sub
}

type sub struct {
	re   *regexp.Regexp
	with string
}

func (r rewrite) apply(s string) string {
	for _, x := range r.subs {
		s = x.re.ReplaceAllString(s, x.with)
	}
	return s
}

// rewrites lists every place a derived number is written down. Each pattern
// matches whatever number is currently there, so the fix is idempotent and a
// changed count needs no edit here.
func rewrites(c counts) []rewrite {
	tools, guides := strconv.Itoa(c.tools), strconv.Itoa(c.guides)
	areaWord, guideWord := word(c.areas), word(c.guides)

	return []rewrite{
		{path: "README.md", subs: []sub{
			// "— [78 tools](docs/TOOLS.md)," in the intro sentence.
			{regexp.MustCompile(`\[\d+ tools\]\(docs/TOOLS\.md\)`), "[" + tools + " tools](docs/TOOLS.md)"},
			// "78 tools across ten areas." opening the Tools section.
			{regexp.MustCompile(`(?m)^\d+ tools across \w+ areas\.`), tools + " tools across " + areaWord + " areas."},
			// "ships as five MCP **resources**".
			{regexp.MustCompile(`ships as \w+ MCP \*\*resources\*\*`), "ships as " + guideWord + " MCP **resources**"},
		}},
		{path: "docs/TOOLS.md", subs: []sub{
			{regexp.MustCompile(`(?m)^\d+ tools \+ \d+ guide resources, across the \w+ areas below\.`),
				tools + " tools + " + guides + " guide resources, across the " + areaWord + " areas below."},
		}},
		// The version is written in code and in the registry manifest too. Both
		// are checked again by the release gate against the git tag, but drift
		// should be impossible before it ever gets that far.
		{path: "cmd/adb-mcp/main.go", subs: []sub{
			{regexp.MustCompile(`(?m)^var version = "[0-9][0-9.]*"$`),
				`var version = "` + c.version + `"`},
		}},
		{path: "server.json", subs: []sub{
			{regexp.MustCompile(`"version": "[0-9][0-9.]*"`),
				`"version": "` + c.version + `"`},
			// The OCI identifier carries the version a second time, in its tag.
			{regexp.MustCompile(`("identifier": "ghcr\.io/[^:"]+):[0-9][0-9.]*"`),
				`${1}:` + c.version + `"`},
		}},
		// Gitignored roadmap: CI can never see it, which is exactly why it went
		// stale twice. Updated locally by `make docs`.
		{path: "docs/personal/TODO.md", optional: true, subs: []sub{
			{regexp.MustCompile(`\*\*Current:\*\* v[0-9][0-9.]* · \d+ tools \+ \d+ guide resources`),
				"**Current:** v" + c.version + " · " + tools + " tools + " + guides + " guide resources"},
			{regexp.MustCompile(`newest first \(v0\.1\.0 → v[0-9][0-9.]*\)`),
				"newest first (v0.1.0 → v" + c.version + ")"},
		}},
	}
}

// word spells small numbers, because the prose says "ten areas", not "10
// areas". Falls back to digits past the range the docs actually use.
func word(n int) string {
	words := []string{"zero", "one", "two", "three", "four", "five", "six", "seven",
		"eight", "nine", "ten", "eleven", "twelve", "thirteen", "fourteen", "fifteen"}
	if n >= 0 && n < len(words) {
		return words[n]
	}
	return strconv.Itoa(n)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "docsync:", err)
	os.Exit(1)
}
