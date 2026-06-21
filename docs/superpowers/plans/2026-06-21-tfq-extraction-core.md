# tfq Extraction Core Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Go extraction core for `tfq` — `Inspect(path) → FileVitals` — that parses frontmatter, links, tags/markers, and headings from any recognized text file and emits a schema-valid JSON contract.

**Architecture:** A data-driven extension→format registry routes a file to a set of modular extractors. Each extractor is a named RE2 regex-set (compatible with both Go `regexp` and ripgrep) plus a small Go post-processing step, returns typed records with positions, and never fails (liberal — collects warnings). The engine orchestrates them into a `FileVitals` struct whose JSON output is gated against a predefined JSON Schema in tests.

**Tech Stack:** Go 1.25, `gopkg.in/yaml.v3` (frontmatter), `github.com/santhosh-tekuri/jsonschema/v6` (output-contract test gate), ripgrep available on PATH (used later; patterns are kept RE2-compatible so they run in-process now and via `rg` later).

## Global Constraints

- Module path: `tfq`. Go version floor: `go 1.25`.
- **Liberal extraction:** no extractor ever returns a fatal error for malformed content; it returns whatever it parsed plus `[]model.Warning`. Only I/O errors (file unreadable) propagate from `engine.Inspect`.
- **Positions are 1-based** (`line` and `col` both start at 1) and **absolute** to the original file.
- **Slices/maps in `FileVitals` are never nil in output** — they marshal as `[]` / `{}`, never `null`. The engine guarantees this.
- **Regex dialect: RE2 only** — no backreferences, no lookahead/lookbehind. This keeps every pattern runnable both in Go and in ripgrep.
- No semantic search, no index, no network at runtime, no full-CUE dependency.
- Every interaction mode's output is validated against a JSON Schema in the test suite. A mode without a passing schema test is not done. (This plan delivers the `file-vitals` mode.)

---

### Task 1: Project bootstrap

**Files:**
- Create: `go.mod`
- Create: `internal/smoke/smoke_test.go` (temporary smoke test, deleted at end of task)

**Interfaces:**
- Produces: a buildable Go module named `tfq` with `gopkg.in/yaml.v3` and `github.com/santhosh-tekuri/jsonschema/v6` available.

- [ ] **Step 1: Initialize the module**

```bash
go mod init tfq
```

- [ ] **Step 2: Add dependencies**

```bash
go get gopkg.in/yaml.v3@latest
go get github.com/santhosh-tekuri/jsonschema/v6@latest
```

Expected: both resolve and appear in `go.mod` / `go.sum`. If the environment has no network, fetch into the module cache first (see Execution Notes at end) — do not proceed until `go list -m all` shows both modules.

- [ ] **Step 3: Write a smoke test that exercises both deps**

```go
// internal/smoke/smoke_test.go
package smoke

import (
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

func TestDepsLink(t *testing.T) {
	var m map[string]any
	if err := yaml.Unmarshal([]byte("a: 1\n"), &m); err != nil {
		t.Fatalf("yaml: %v", err)
	}
	if m["a"] != 1 {
		t.Fatalf("yaml parse wrong: %v", m["a"])
	}
	if c := jsonschema.NewCompiler(); c == nil {
		t.Fatal("jsonschema compiler nil")
	}
}
```

- [ ] **Step 4: Run it**

Run: `go test ./internal/smoke/...`
Expected: PASS.

- [ ] **Step 5: Delete the smoke test and commit**

```bash
rm -rf internal/smoke
git add go.mod go.sum
git commit -m "chore: bootstrap go module with yaml + jsonschema deps"
```

---

### Task 2: Core model types

**Files:**
- Create: `internal/model/model.go`
- Test: `internal/model/model_test.go`

**Interfaces:**
- Produces:
  - `model.Position{Line int, Col int}` (currently unused directly but reserved)
  - `model.Heading{Level int, Text string, Line int}`
  - `model.Link{Kind string, Target string, Label *string, Line int, Col int}`
  - `model.Marker{Kind string, Value string, Line int, Col int}`
  - `model.Warning{Module string, Message string}`
  - `model.FileVitals{Path, Ext, Format string, Frontmatter map[string]any, Headings []Heading, Links []Link, Markers []Marker, Warnings []Warning}`
  - Link kind constants: `LinkMarkdown, LinkWiki, LinkEmbed, LinkOrg, LinkAutolink, LinkBareURL`
  - Marker kind constants: `MarkerHashtag, MarkerOrgTag, MarkerAngle, MarkerDoubleAngle`

- [ ] **Step 1: Write the failing test**

```go
// internal/model/model_test.go
package model

import (
	"encoding/json"
	"testing"
)

func TestFileVitalsJSONKeys(t *testing.T) {
	label := "alias"
	fv := FileVitals{
		Path:        "a.md",
		Ext:         ".md",
		Format:      "markdown",
		Frontmatter: map[string]any{"k": "v"},
		Headings:    []Heading{{Level: 1, Text: "T", Line: 3}},
		Links:       []Link{{Kind: LinkWiki, Target: "x", Label: &label, Line: 8, Col: 4}},
		Markers:     []Marker{{Kind: MarkerHashtag, Value: "tag", Line: 9, Col: 1}},
		Warnings:    []Warning{},
	}
	b, err := json.Marshal(fv)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"path", "ext", "format", "frontmatter", "headings", "links", "markers", "warnings"} {
		if _, ok := got[k]; !ok {
			t.Errorf("missing key %q in %s", k, b)
		}
	}
	// label must serialize as a string, not be dropped
	links := got["links"].([]any)
	if links[0].(map[string]any)["label"] != "alias" {
		t.Errorf("label wrong: %v", links[0])
	}
}

func TestNilLabelSerializesNull(t *testing.T) {
	b, _ := json.Marshal(Link{Kind: LinkBareURL, Target: "http://x", Line: 1, Col: 1})
	var got map[string]any
	_ = json.Unmarshal(b, &got)
	if v, ok := got["label"]; !ok || v != nil {
		t.Errorf("nil label should serialize as JSON null, got %v ok=%v", v, ok)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/...`
Expected: FAIL (package/types not defined).

- [ ] **Step 3: Write the implementation**

```go
// internal/model/model.go
package model

// Link kinds.
const (
	LinkMarkdown = "markdown"
	LinkWiki     = "wiki"
	LinkEmbed    = "embed"
	LinkOrg      = "org"
	LinkAutolink = "autolink"
	LinkBareURL  = "bare-url"
)

// Marker kinds.
const (
	MarkerHashtag     = "hashtag"
	MarkerOrgTag      = "org-tag"
	MarkerAngle       = "angle"
	MarkerDoubleAngle = "double-angle"
)

// Position is a 1-based line/column location.
type Position struct {
	Line int `json:"line"`
	Col  int `json:"col"`
}

// Heading is a section heading (markdown # or org *).
type Heading struct {
	Level int    `json:"level"`
	Text  string `json:"text"`
	Line  int    `json:"line"`
}

// Link is any cross-reference found in the body. Label is nil when absent.
type Link struct {
	Kind   string  `json:"kind"`
	Target string  `json:"target"`
	Label  *string `json:"label"`
	Line   int     `json:"line"`
	Col    int     `json:"col"`
}

// Marker is a hashtag, org tag, or angle-bracket phrase.
type Marker struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
	Line  int    `json:"line"`
	Col   int    `json:"col"`
}

// Warning is a non-fatal extraction issue.
type Warning struct {
	Module  string `json:"module"`
	Message string `json:"message"`
}

// FileVitals is the comprehensive per-file output contract.
type FileVitals struct {
	Path        string         `json:"path"`
	Ext         string         `json:"ext"`
	Format      string         `json:"format"`
	Frontmatter map[string]any `json:"frontmatter"`
	Headings    []Heading      `json:"headings"`
	Links       []Link         `json:"links"`
	Markers     []Marker       `json:"markers"`
	Warnings    []Warning      `json:"warnings"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/model/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/model
git commit -m "feat(model): FileVitals and record types with JSON contract"
```

---

### Task 3: Position helper (`lineCol`)

**Files:**
- Create: `internal/extract/util.go`
- Test: `internal/extract/util_test.go`

**Interfaces:**
- Produces: `func lineCol(content string, byteOffset int) (line, col int)` (package-private, used by all extractors). 1-based; `col` counts bytes from the start of the line.

- [ ] **Step 1: Write the failing test**

```go
// internal/extract/util_test.go
package extract

import "testing"

func TestLineCol(t *testing.T) {
	c := "ab\ncde\nf"
	cases := []struct {
		off, line, col int
	}{
		{0, 1, 1},  // 'a'
		{1, 1, 2},  // 'b'
		{3, 2, 1},  // 'c' (after first \n)
		{5, 2, 3},  // 'e'
		{7, 3, 1},  // 'f'
	}
	for _, tc := range cases {
		l, col := lineCol(c, tc.off)
		if l != tc.line || col != tc.col {
			t.Errorf("off=%d got (%d,%d) want (%d,%d)", tc.off, l, col, tc.line, tc.col)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/extract/... -run TestLineCol`
Expected: FAIL (`lineCol` undefined).

- [ ] **Step 3: Write the implementation**

```go
// internal/extract/util.go
package extract

import "strings"

// lineCol converts a byte offset into a 1-based (line, col).
// col is the byte position within the line, also 1-based.
func lineCol(content string, byteOffset int) (int, int) {
	if byteOffset < 0 {
		byteOffset = 0
	}
	if byteOffset > len(content) {
		byteOffset = len(content)
	}
	prefix := content[:byteOffset]
	line := strings.Count(prefix, "\n") + 1
	col := byteOffset - (strings.LastIndex(prefix, "\n") + 1) + 1
	return line, col
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/extract/... -run TestLineCol`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/extract/util.go internal/extract/util_test.go
git commit -m "feat(extract): 1-based lineCol position helper"
```

---

### Task 4: Frontmatter extractor

**Files:**
- Create: `internal/extract/frontmatter.go`
- Test: `internal/extract/frontmatter_test.go`

**Interfaces:**
- Consumes: `gopkg.in/yaml.v3`, `model.Warning`.
- Produces: `func Frontmatter(content string) (fm map[string]any, body string, warnings []model.Warning)`.
  - `fm` is the parsed YAML map, or an empty map if there is no frontmatter or it failed to parse.
  - `body` is `content` with the leading `---`…`---` block replaced by blank lines (same line count preserved) so downstream positions stay absolute. If there is no frontmatter, `body == content`.
  - Malformed YAML → empty `fm` + one warning, never an error.

- [ ] **Step 1: Write the failing test**

```go
// internal/extract/frontmatter_test.go
package extract

import "testing"

func TestFrontmatterParses(t *testing.T) {
	c := "---\ntitle: Hi\ntags: [a, b]\n---\n# Heading\nbody\n"
	fm, body, warns := Frontmatter(c)
	if len(warns) != 0 {
		t.Fatalf("unexpected warnings: %v", warns)
	}
	if fm["title"] != "Hi" {
		t.Errorf("title = %v", fm["title"])
	}
	// body must preserve line count: heading was on line 5, still line 5
	l, _ := lineCol(body, indexOf(body, "# Heading"))
	if l != 5 {
		t.Errorf("heading line = %d, want 5", l)
	}
	// frontmatter region blanked
	if containsLine(body, "title: Hi") {
		t.Errorf("frontmatter not blanked: %q", body)
	}
}

func TestFrontmatterNone(t *testing.T) {
	c := "# Just a heading\nno frontmatter\n"
	fm, body, warns := Frontmatter(c)
	if len(fm) != 0 {
		t.Errorf("expected empty fm, got %v", fm)
	}
	if body != c {
		t.Errorf("body should be unchanged")
	}
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
}

func TestFrontmatterMalformed(t *testing.T) {
	c := "---\ntitle: : : bad\n  - nope\n---\nbody\n"
	fm, _, warns := Frontmatter(c)
	if len(fm) != 0 {
		t.Errorf("malformed fm should yield empty map, got %v", fm)
	}
	if len(warns) == 0 {
		t.Errorf("expected a warning for malformed yaml")
	}
}

// test helpers
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
func containsLine(s, sub string) bool { return indexOf(s, sub) >= 0 }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/extract/... -run TestFrontmatter`
Expected: FAIL (`Frontmatter` undefined).

- [ ] **Step 3: Write the implementation**

```go
// internal/extract/frontmatter.go
package extract

import (
	"strings"

	"tfq/internal/model"
	"gopkg.in/yaml.v3"
)

// Frontmatter extracts a leading YAML frontmatter block delimited by lines
// containing only "---". It returns the parsed map, the body with the
// frontmatter region blanked (line count preserved), and any warnings.
func Frontmatter(content string) (map[string]any, string, []model.Warning) {
	empty := map[string]any{}
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r") != "---" {
		return empty, content, nil
	}
	// find the closing fence
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r") == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		// no closing fence: treat as no frontmatter (liberal)
		return empty, content, []model.Warning{{Module: "frontmatter", Message: "opening --- without closing ---; ignored"}}
	}
	yamlSrc := strings.Join(lines[1:end], "\n")

	var fm map[string]any
	var warns []model.Warning
	if err := yaml.Unmarshal([]byte(yamlSrc), &fm); err != nil || fm == nil {
		if err != nil {
			warns = append(warns, model.Warning{Module: "frontmatter", Message: "yaml parse error: " + err.Error()})
		}
		fm = map[string]any{}
	}

	// blank lines 0..end inclusive to preserve absolute line numbers downstream
	blanked := make([]string, len(lines))
	for i := range lines {
		if i <= end {
			blanked[i] = ""
		} else {
			blanked[i] = lines[i]
		}
	}
	return fm, strings.Join(blanked, "\n"), warns
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/extract/... -run TestFrontmatter`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/extract/frontmatter.go internal/extract/frontmatter_test.go
git commit -m "feat(extract): liberal YAML frontmatter extractor with body blanking"
```

---

### Task 5: Headings extractor

**Files:**
- Create: `internal/extract/headings.go`
- Test: `internal/extract/headings_test.go`

**Interfaces:**
- Consumes: `model.Heading`, `model.Warning`, `lineCol`.
- Produces: `func Headings(content, format string) ([]model.Heading, []model.Warning)`.
  - `format == "org"`: heading is `^\*+\s+TEXT` (level = count of `*`).
  - otherwise (markdown/text/unknown): heading is `^#{1,6}\s+TEXT` (level = count of `#`).
  - `Line` is 1-based; trailing whitespace trimmed from `Text`.

- [ ] **Step 1: Write the failing test**

```go
// internal/extract/headings_test.go
package extract

import "testing"

func TestHeadingsMarkdown(t *testing.T) {
	c := "# One\ntext\n### Three\n#nospace\n"
	hs, _ := Headings(c, "markdown")
	if len(hs) != 2 {
		t.Fatalf("got %d headings: %#v", len(hs), hs)
	}
	if hs[0].Level != 1 || hs[0].Text != "One" || hs[0].Line != 1 {
		t.Errorf("h0 = %#v", hs[0])
	}
	if hs[1].Level != 3 || hs[1].Text != "Three" || hs[1].Line != 3 {
		t.Errorf("h1 = %#v", hs[1])
	}
}

func TestHeadingsOrg(t *testing.T) {
	c := "* One\n** Two\n# not a heading in org\n"
	hs, _ := Headings(c, "org")
	if len(hs) != 2 {
		t.Fatalf("got %d: %#v", len(hs), hs)
	}
	if hs[1].Level != 2 || hs[1].Text != "Two" {
		t.Errorf("h1 = %#v", hs[1])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/extract/... -run TestHeadings`
Expected: FAIL (`Headings` undefined).

- [ ] **Step 3: Write the implementation**

```go
// internal/extract/headings.go
package extract

import (
	"regexp"
	"strings"

	"tfq/internal/model"
)

var (
	reMdHeading  = regexp.MustCompile(`(?m)^(#{1,6})\s+(.*\S)\s*$`)
	reOrgHeading = regexp.MustCompile(`(?m)^(\*+)\s+(.*\S)\s*$`)
)

// Headings extracts section headings. Org files use leading '*'; everything
// else uses markdown '#'. Never fails.
func Headings(content, format string) ([]model.Heading, []model.Warning) {
	re := reMdHeading
	marker := "#"
	if format == "org" {
		re = reOrgHeading
		marker = "*"
	}
	out := []model.Heading{}
	for _, m := range re.FindAllStringSubmatchIndex(content, -1) {
		full := content[m[0]:m[1]]
		level := strings.Count(full[:strings.IndexFunc(full, func(r rune) bool {
			return string(r) != marker
		})], marker)
		text := strings.TrimSpace(content[m[4]:m[5]])
		line, _ := lineCol(content, m[0])
		out = append(out, model.Heading{Level: level, Text: text, Line: line})
	}
	return out, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/extract/... -run TestHeadings`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/extract/headings.go internal/extract/headings_test.go
git commit -m "feat(extract): format-aware heading extractor (md + org)"
```

---

### Task 6: Markers extractor (tags + angle brackets)

**Files:**
- Create: `internal/extract/markers.go`
- Test: `internal/extract/markers_test.go`

**Interfaces:**
- Consumes: `model.Marker`, `model.Warning`, `lineCol`.
- Produces: `func Markers(content, format string) ([]model.Marker, []model.Warning)`.
  - Always: `#hashtag` (`MarkerHashtag`), `<<phrase>>` (`MarkerDoubleAngle`), `<phrase>` (`MarkerAngle`).
  - Org format only: `:tag:tag2:` (`MarkerOrgTag`) — each colon-delimited tag emitted separately.
  - Double-angle must win over single-angle on the same text (no double counting).
  - Angle markers whose value looks like a URL/autolink (`scheme://` or `mailto:`) are **skipped** (those are links, handled by Task 7).
  - Hashtag value excludes the leading `#`; org-tag value excludes colons; angle values exclude the brackets.

- [ ] **Step 1: Write the failing test**

```go
// internal/extract/markers_test.go
package extract

import "testing"

func has(ms []Markerish, kind, val string) bool {
	for _, m := range ms {
		if m.Kind == kind && m.Value == val {
			return true
		}
	}
	return false
}

// Markerish mirrors model.Marker for terse assertions.
type Markerish struct {
	Kind, Value string
}

func toMarkerish(in any) []Markerish {
	out := []Markerish{}
	for _, m := range in.([]struct {
		Kind, Value string
		Line, Col   int
	}) {
		out = append(out, Markerish{m.Kind, m.Value})
	}
	return out
}

func TestMarkersHashtagsAndAngles(t *testing.T) {
	c := "intro #alpha and <single phrase> then <<double phrase>>\n"
	ms, _ := Markers(c, "markdown")
	got := []Markerish{}
	for _, m := range ms {
		got = append(got, Markerish{m.Kind, m.Value})
	}
	if !has(got, "hashtag", "alpha") {
		t.Errorf("missing hashtag alpha: %v", got)
	}
	if !has(got, "double-angle", "double phrase") {
		t.Errorf("missing double-angle: %v", got)
	}
	if !has(got, "angle", "single phrase") {
		t.Errorf("missing single angle: %v", got)
	}
	// the inside of a << >> must NOT also be reported as a single angle
	if has(got, "angle", "double phrase") {
		t.Errorf("double-angle double-counted as angle: %v", got)
	}
}

func TestMarkersOrgTagsOnlyInOrg(t *testing.T) {
	c := "* Heading :work:urgent:\n"
	org, _ := Markers(c, "org")
	g := []Markerish{}
	for _, m := range org {
		g = append(g, Markerish{m.Kind, m.Value})
	}
	if !has(g, "org-tag", "work") || !has(g, "org-tag", "urgent") {
		t.Errorf("missing org tags: %v", g)
	}
	md, _ := Markers(c, "markdown")
	for _, m := range md {
		if m.Kind == "org-tag" {
			t.Errorf("org tags should not fire in markdown: %v", m)
		}
	}
}

func TestMarkersSkipURLAngles(t *testing.T) {
	c := "see <https://example.com> for more\n"
	ms, _ := Markers(c, "markdown")
	for _, m := range ms {
		if m.Kind == "angle" {
			t.Errorf("url autolink should not be an angle marker: %v", m)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/extract/... -run TestMarkers`
Expected: FAIL (`Markers` undefined).

- [ ] **Step 3: Write the implementation**

```go
// internal/extract/markers.go
package extract

import (
	"regexp"
	"strings"

	"tfq/internal/model"
)

var (
	reHashtag     = regexp.MustCompile(`(^|[^\w&])#([A-Za-z0-9][\w/-]*)`)
	reDoubleAngle = regexp.MustCompile(`<<([^<>]+)>>`)
	reSingleAngle = regexp.MustCompile(`<([^<>]+)>`)
	reOrgTagLine  = regexp.MustCompile(`(?m)(\s):([A-Za-z0-9_@%][A-Za-z0-9_@%:]*):\s*$`)
	reURLish      = regexp.MustCompile(`^(?:[a-zA-Z][a-zA-Z0-9+.-]*://|mailto:)`)
)

// Markers extracts hashtags, angle-bracket phrases, and (org only) org tags.
// Never fails.
func Markers(content, format string) ([]model.Marker, []model.Warning) {
	out := []model.Marker{}
	consumed := map[int]bool{} // byte offsets covered by an accepted marker

	// hashtags
	for _, m := range reHashtag.FindAllStringSubmatchIndex(content, -1) {
		start, end := m[4], m[5] // group 2 (without leading boundary / '#')
		line, col := lineCol(content, start-1) // point at the '#'
		out = append(out, model.Marker{Kind: model.MarkerHashtag, Value: content[start:end], Line: line, Col: col})
	}

	// double-angle first (claims its byte range)
	for _, m := range reDoubleAngle.FindAllStringSubmatchIndex(content, -1) {
		for i := m[0]; i < m[1]; i++ {
			consumed[i] = true
		}
		line, col := lineCol(content, m[0])
		out = append(out, model.Marker{Kind: model.MarkerDoubleAngle, Value: content[m[2]:m[3]], Line: line, Col: col})
	}

	// single-angle, skipping ranges already consumed by << >> and url-ish values
	for _, m := range reSingleAngle.FindAllStringSubmatchIndex(content, -1) {
		if consumed[m[0]] || consumed[m[1]-1] {
			continue
		}
		val := content[m[2]:m[3]]
		if reURLish.MatchString(val) {
			continue
		}
		line, col := lineCol(content, m[0])
		out = append(out, model.Marker{Kind: model.MarkerAngle, Value: val, Line: line, Col: col})
	}

	// org tags (org format only)
	if format == "org" {
		for _, m := range reOrgTagLine.FindAllStringSubmatchIndex(content, -1) {
			group := content[m[4]:m[5]] // "work:urgent"
			base := m[4]
			offset := 0
			for _, tag := range strings.Split(group, ":") {
				if tag == "" {
					offset += 1
					continue
				}
				line, col := lineCol(content, base+offset)
				out = append(out, model.Marker{Kind: model.MarkerOrgTag, Value: tag, Line: line, Col: col})
				offset += len(tag) + 1
			}
		}
	}

	return out, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/extract/... -run TestMarkers`
Expected: PASS. (If the `toMarkerish`/`has` helpers in the test conflict, simplify the test to the inline `got` loops shown — the canonical assertions are the `TestMarkers*` functions; delete the unused `toMarkerish` helper.)

- [ ] **Step 5: Commit**

```bash
git add internal/extract/markers.go internal/extract/markers_test.go
git commit -m "feat(extract): markers extractor (hashtags, angles, org tags)"
```

---

### Task 7: Links extractor (liberal, overlap-resolved)

**Files:**
- Create: `internal/extract/links.go`
- Test: `internal/extract/links_test.go`
- Test: `internal/extract/testdata/links_corpus.md` (conformance corpus)

**Interfaces:**
- Consumes: `model.Link`, `model.Warning`, `lineCol`.
- Produces: `func Links(content string) ([]model.Link, []model.Warning)`.
  - Recognizes (in priority order, longest/most-specific wins on overlap):
    1. embed `![[target]]` / `![[target|alias]]` → `LinkEmbed`
    2. org `[[link][desc]]` → `LinkOrg` (Label = desc)
    3. wiki `[[target]]` / `[[target|alias]]` → `LinkWiki`
    4. markdown `[label](target)` → `LinkMarkdown`
    5. autolink `<scheme://…>` / `<mailto:…>` → `LinkAutolink`
    6. bare url `scheme://…` → `LinkBareURL`
  - Overlap resolution: collect all candidate matches with byte ranges, sort by start asc then by priority, greedily accept a candidate only if its range does not overlap an already-accepted one.
  - `Label` is nil unless an alias/description is present.

- [ ] **Step 1: Write the conformance corpus and failing test**

```text
<!-- internal/extract/testdata/links_corpus.md -->
embed: ![[image.png]]
embed alias: ![[note|Nice Note]]
org: [[https://o.example][Org Desc]]
wiki: [[Plain Note]]
wiki alias: [[Target|Shown]]
markdown: [Click here](https://md.example/page)
autolink: <https://auto.example>
bare: see https://bare.example/x for details
```

```go
// internal/extract/links_test.go
package extract

import (
	"os"
	"testing"
)

func find(ls []Linkish, kind, target string) *Linkish {
	for i := range ls {
		if ls[i].Kind == kind && ls[i].Target == target {
			return &ls[i]
		}
	}
	return nil
}

type Linkish struct {
	Kind, Target string
	Label        *string
}

func TestLinksCorpus(t *testing.T) {
	b, err := os.ReadFile("testdata/links_corpus.md")
	if err != nil {
		t.Fatal(err)
	}
	links, _ := Links(string(b))
	ls := []Linkish{}
	for _, l := range links {
		ls = append(ls, Linkish{l.Kind, l.Target, l.Label})
	}

	if find(ls, "embed", "image.png") == nil {
		t.Errorf("missing embed image.png: %#v", ls)
	}
	if e := find(ls, "embed", "note"); e == nil || e.Label == nil || *e.Label != "Nice Note" {
		t.Errorf("embed alias wrong: %#v", ls)
	}
	if o := find(ls, "org", "https://o.example"); o == nil || o.Label == nil || *o.Label != "Org Desc" {
		t.Errorf("org link wrong: %#v", ls)
	}
	if find(ls, "wiki", "Plain Note") == nil {
		t.Errorf("missing wiki Plain Note: %#v", ls)
	}
	if w := find(ls, "wiki", "Target"); w == nil || w.Label == nil || *w.Label != "Shown" {
		t.Errorf("wiki alias wrong: %#v", ls)
	}
	if m := find(ls, "markdown", "https://md.example/page"); m == nil || m.Label == nil || *m.Label != "Click here" {
		t.Errorf("markdown link wrong: %#v", ls)
	}
	if find(ls, "autolink", "https://auto.example") == nil {
		t.Errorf("missing autolink: %#v", ls)
	}
	if find(ls, "bare-url", "https://bare.example/x") == nil {
		t.Errorf("missing bare url: %#v", ls)
	}
	// the bare url inside the markdown link target must NOT be double-counted
	count := 0
	for _, l := range ls {
		if l.Target == "https://md.example/page" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("md target double counted: %d", count)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/extract/... -run TestLinks`
Expected: FAIL (`Links` undefined).

- [ ] **Step 3: Write the implementation**

```go
// internal/extract/links.go
package extract

import (
	"regexp"
	"sort"

	"tfq/internal/model"
)

type linkPattern struct {
	re       *regexp.Regexp
	kind     string
	priority int // lower = higher priority
	build    func(content string, m []int) model.Link
}

func strptr(s string) *string { return &s }

var linkPatterns = []linkPattern{
	{ // embed
		re:       regexp.MustCompile(`!\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`),
		kind:     model.LinkEmbed,
		priority: 0,
		build: func(c string, m []int) model.Link {
			var label *string
			if m[4] >= 0 {
				label = strptr(c[m[4]:m[5]])
			}
			return model.Link{Kind: model.LinkEmbed, Target: c[m[2]:m[3]], Label: label}
		},
	},
	{ // org [[link][desc]]
		re:       regexp.MustCompile(`\[\[([^\]]+)\]\[([^\]]+)\]\]`),
		kind:     model.LinkOrg,
		priority: 1,
		build: func(c string, m []int) model.Link {
			return model.Link{Kind: model.LinkOrg, Target: c[m[2]:m[3]], Label: strptr(c[m[4]:m[5]])}
		},
	},
	{ // wiki [[target]] / [[target|alias]]
		re:       regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`),
		kind:     model.LinkWiki,
		priority: 2,
		build: func(c string, m []int) model.Link {
			var label *string
			if m[4] >= 0 {
				label = strptr(c[m[4]:m[5]])
			}
			return model.Link{Kind: model.LinkWiki, Target: c[m[2]:m[3]], Label: label}
		},
	},
	{ // markdown [label](target)
		re:       regexp.MustCompile(`\[([^\]]*)\]\(([^)\s]+)\)`),
		kind:     model.LinkMarkdown,
		priority: 3,
		build: func(c string, m []int) model.Link {
			return model.Link{Kind: model.LinkMarkdown, Target: c[m[4]:m[5]], Label: strptr(c[m[2]:m[3]])}
		},
	},
	{ // autolink <scheme://...> or <mailto:...>
		re:       regexp.MustCompile(`<((?:[a-zA-Z][a-zA-Z0-9+.-]*://|mailto:)[^>\s]+)>`),
		kind:     model.LinkAutolink,
		priority: 4,
		build: func(c string, m []int) model.Link {
			return model.Link{Kind: model.LinkAutolink, Target: c[m[2]:m[3]]}
		},
	},
	{ // bare url
		re:       regexp.MustCompile(`(?:[a-zA-Z][a-zA-Z0-9+.-]*://)[^\s)>\]]+`),
		kind:     model.LinkBareURL,
		priority: 5,
		build: func(c string, m []int) model.Link {
			return model.Link{Kind: model.LinkBareURL, Target: c[m[0]:m[1]]}
		},
	},
}

type candidate struct {
	start, end, priority int
	link                 model.Link
}

// Links extracts all recognized link forms with overlap resolution.
// Never fails.
func Links(content string) ([]model.Link, []model.Warning) {
	cands := []candidate{}
	for _, p := range linkPatterns {
		for _, m := range p.re.FindAllStringSubmatchIndex(content, -1) {
			l := p.build(content, m)
			line, col := lineCol(content, m[0])
			l.Line, l.Col = line, col
			cands = append(cands, candidate{start: m[0], end: m[1], priority: p.priority, link: l})
		}
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].start != cands[j].start {
			return cands[i].start < cands[j].start
		}
		return cands[i].priority < cands[j].priority
	})
	out := []model.Link{}
	occupied := []candidate{}
	overlaps := func(a, b candidate) bool { return a.start < b.end && b.start < a.end }
	for _, c := range cands {
		clash := false
		for _, o := range occupied {
			if overlaps(c, o) {
				clash = true
				break
			}
		}
		if clash {
			continue
		}
		occupied = append(occupied, c)
		out = append(out, c.link)
	}
	// stable output ordering by position
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Line != out[j].Line {
			return out[i].Line < out[j].Line
		}
		return out[i].Col < out[j].Col
	})
	return out, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/extract/... -run TestLinks`
Expected: PASS.

- [ ] **Step 5: Run the full extract package and commit**

Run: `go test ./internal/extract/...`
Expected: PASS (all extractor tests).

```bash
git add internal/extract/links.go internal/extract/links_test.go internal/extract/testdata
git commit -m "feat(extract): liberal link parser with overlap resolution + corpus"
```

---

### Task 8: Registry (extension → format)

**Files:**
- Create: `internal/registry/registry.go`
- Test: `internal/registry/registry_test.go`

**Interfaces:**
- Produces: `func FormatFor(ext string) string`.
  - `.md`, `.markdown` → `"markdown"`; `.org` → `"org"`; anything else → `"text"`. Case-insensitive; accepts ext with or without leading dot.

- [ ] **Step 1: Write the failing test**

```go
// internal/registry/registry_test.go
package registry

import "testing"

func TestFormatFor(t *testing.T) {
	cases := map[string]string{
		".md":       "markdown",
		".markdown": "markdown",
		"md":        "markdown",
		".MD":       "markdown",
		".org":      "org",
		".txt":      "text",
		"":          "text",
		".rst":      "text",
	}
	for in, want := range cases {
		if got := FormatFor(in); got != want {
			t.Errorf("FormatFor(%q) = %q, want %q", in, got, want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/registry/...`
Expected: FAIL.

- [ ] **Step 3: Write the implementation**

```go
// internal/registry/registry.go
package registry

import "strings"

// FormatFor maps a file extension (with or without leading dot, any case) to
// a format name. Unknown extensions fall back to "text".
func FormatFor(ext string) string {
	e := strings.ToLower(strings.TrimPrefix(ext, "."))
	switch e {
	case "md", "markdown":
		return "markdown"
	case "org":
		return "org"
	default:
		return "text"
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/registry/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/registry
git commit -m "feat(registry): extension to format mapping"
```

---

### Task 9: Engine (`Inspect`)

**Files:**
- Create: `internal/engine/engine.go`
- Test: `internal/engine/engine_test.go`
- Test fixtures: `internal/engine/testdata/note.md`, `internal/engine/testdata/tasks.org`

**Interfaces:**
- Consumes: `registry.FormatFor`, `extract.Frontmatter/Headings/Markers/Links`, `model.FileVitals`.
- Produces:
  - `func InspectContent(path, content string) model.FileVitals` — pure, no I/O; **guarantees non-nil** Frontmatter/Headings/Links/Markers/Warnings.
  - `func Inspect(path string) (model.FileVitals, error)` — reads the file then calls `InspectContent`; only I/O errors propagate.

- [ ] **Step 1: Write fixtures and failing test**

```text
<!-- internal/engine/testdata/note.md -->
---
title: Bandgap Notes
tags: [bandgap, sim]
---
# Bandgap synthesis

PTAT current tracking well. See [[../tasks/001-review]] and #followup.
Reference <https://example.com/datasheet>.
```

```text
# internal/engine/testdata/tasks.org
* TODO Review sim :work:urgent:
Link to [[file:notes.org][Notes]].
```

```go
// internal/engine/engine_test.go
package engine

import "testing"

func TestInspectMarkdown(t *testing.T) {
	fv, err := Inspect("testdata/note.md")
	if err != nil {
		t.Fatal(err)
	}
	if fv.Format != "markdown" || fv.Ext != ".md" {
		t.Errorf("format/ext wrong: %s %s", fv.Format, fv.Ext)
	}
	if fv.Frontmatter["title"] != "Bandgap Notes" {
		t.Errorf("frontmatter title: %v", fv.Frontmatter["title"])
	}
	if len(fv.Headings) != 1 || fv.Headings[0].Text != "Bandgap synthesis" {
		t.Errorf("headings: %#v", fv.Headings)
	}
	foundWiki, foundFollowup := false, false
	for _, l := range fv.Links {
		if l.Kind == "wiki" && l.Target == "../tasks/001-review" {
			foundWiki = true
		}
	}
	for _, m := range fv.Markers {
		if m.Kind == "hashtag" && m.Value == "followup" {
			foundFollowup = true
		}
	}
	if !foundWiki {
		t.Errorf("wiki link not found: %#v", fv.Links)
	}
	if !foundFollowup {
		t.Errorf("#followup not found: %#v", fv.Markers)
	}
}

func TestInspectOrgTags(t *testing.T) {
	fv, err := Inspect("testdata/tasks.org")
	if err != nil {
		t.Fatal(err)
	}
	if fv.Format != "org" {
		t.Errorf("format: %s", fv.Format)
	}
	found := false
	for _, m := range fv.Markers {
		if m.Kind == "org-tag" && m.Value == "urgent" {
			found = true
		}
	}
	if !found {
		t.Errorf("org tag urgent not found: %#v", fv.Markers)
	}
}

func TestInspectContentNeverNil(t *testing.T) {
	fv := InspectContent("empty.md", "")
	if fv.Frontmatter == nil || fv.Headings == nil || fv.Links == nil || fv.Markers == nil || fv.Warnings == nil {
		t.Errorf("nil slice/map in output: %#v", fv)
	}
}

func TestInspectMissingFile(t *testing.T) {
	if _, err := Inspect("testdata/does-not-exist.md"); err == nil {
		t.Error("expected I/O error for missing file")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/engine/...`
Expected: FAIL (`Inspect`/`InspectContent` undefined).

- [ ] **Step 3: Write the implementation**

```go
// internal/engine/engine.go
package engine

import (
	"os"
	"path/filepath"

	"tfq/internal/extract"
	"tfq/internal/model"
	"tfq/internal/registry"
)

// InspectContent runs all extractors over already-loaded content. Pure (no I/O).
// All slices/maps in the result are guaranteed non-nil.
func InspectContent(path, content string) model.FileVitals {
	ext := filepath.Ext(path)
	format := registry.FormatFor(ext)

	fm, body, warns := extract.Frontmatter(content)
	headings, hw := extract.Headings(body, format)
	markers, mw := extract.Markers(body, format)
	links, lw := extract.Links(body)

	allWarn := []model.Warning{}
	allWarn = append(allWarn, warns...)
	allWarn = append(allWarn, hw...)
	allWarn = append(allWarn, mw...)
	allWarn = append(allWarn, lw...)

	if fm == nil {
		fm = map[string]any{}
	}
	if headings == nil {
		headings = []model.Heading{}
	}
	if links == nil {
		links = []model.Link{}
	}
	if markers == nil {
		markers = []model.Marker{}
	}

	return model.FileVitals{
		Path:        path,
		Ext:         ext,
		Format:      format,
		Frontmatter: fm,
		Headings:    headings,
		Links:       links,
		Markers:     markers,
		Warnings:    allWarn,
	}
}

// Inspect reads path and returns its FileVitals. Only I/O errors propagate.
func Inspect(path string) (model.FileVitals, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return model.FileVitals{}, err
	}
	return InspectContent(path, string(b)), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/engine/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/engine
git commit -m "feat(engine): Inspect orchestrates extractors into FileVitals"
```

---

### Task 10: Output JSON Schema + schema-gate test

**Files:**
- Create: `internal/schema/filevitals.schema.json`
- Create: `internal/schema/schema.go`
- Test: `internal/schema/schema_test.go`

**Interfaces:**
- Consumes: `github.com/santhosh-tekuri/jsonschema/v6`, `model.FileVitals`, `engine.InspectContent`.
- Produces:
  - `//go:embed filevitals.schema.json` → `var FileVitalsSchema []byte`
  - `func ValidateFileVitals(fv model.FileVitals) error` — marshals `fv` to JSON and validates against the embedded schema; returns a non-nil error listing violations on failure.

- [ ] **Step 1: Write the JSON Schema**

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://tfq/schemas/filevitals.json",
  "title": "FileVitals",
  "type": "object",
  "additionalProperties": false,
  "required": ["path", "ext", "format", "frontmatter", "headings", "links", "markers", "warnings"],
  "properties": {
    "path": { "type": "string" },
    "ext": { "type": "string" },
    "format": { "type": "string", "enum": ["markdown", "org", "text"] },
    "frontmatter": { "type": "object" },
    "headings": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["level", "text", "line"],
        "properties": {
          "level": { "type": "integer", "minimum": 1 },
          "text": { "type": "string" },
          "line": { "type": "integer", "minimum": 1 }
        }
      }
    },
    "links": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["kind", "target", "label", "line", "col"],
        "properties": {
          "kind": { "type": "string", "enum": ["markdown", "wiki", "embed", "org", "autolink", "bare-url"] },
          "target": { "type": "string" },
          "label": { "type": ["string", "null"] },
          "line": { "type": "integer", "minimum": 1 },
          "col": { "type": "integer", "minimum": 1 }
        }
      }
    },
    "markers": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["kind", "value", "line", "col"],
        "properties": {
          "kind": { "type": "string", "enum": ["hashtag", "org-tag", "angle", "double-angle"] },
          "value": { "type": "string" },
          "line": { "type": "integer", "minimum": 1 },
          "col": { "type": "integer", "minimum": 1 }
        }
      }
    },
    "warnings": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["module", "message"],
        "properties": {
          "module": { "type": "string" },
          "message": { "type": "string" }
        }
      }
    }
  }
}
```

- [ ] **Step 2: Write the failing test**

```go
// internal/schema/schema_test.go
package schema

import (
	"os"
	"path/filepath"
	"testing"

	"tfq/internal/engine"
)

func TestSchemaItself(t *testing.T) {
	if len(FileVitalsSchema) == 0 {
		t.Fatal("embedded schema is empty")
	}
}

func TestEngineOutputMatchesSchema(t *testing.T) {
	// gate every engine fixture through the schema
	roots := []string{
		filepath.Join("..", "engine", "testdata"),
	}
	checked := 0
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatalf("read fixtures: %v", err)
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			b, err := os.ReadFile(filepath.Join(root, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			fv := engine.InspectContent(e.Name(), string(b))
			if err := ValidateFileVitals(fv); err != nil {
				t.Errorf("%s: schema violation: %v", e.Name(), err)
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("no fixtures checked")
	}
}

func TestValidateCatchesBadOutput(t *testing.T) {
	// hand-build an invalid FileVitals (bad format enum) and confirm rejection
	bad := `{"path":"x","ext":".md","format":"WRONG","frontmatter":{},"headings":[],"links":[],"markers":[],"warnings":[]}`
	if err := validateJSON([]byte(bad)); err == nil {
		t.Error("expected schema rejection for bad format enum")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/schema/...`
Expected: FAIL (`FileVitalsSchema`, `ValidateFileVitals`, `validateJSON` undefined).

- [ ] **Step 4: Write the implementation**

```go
// internal/schema/schema.go
package schema

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"tfq/internal/model"
)

//go:embed filevitals.schema.json
var FileVitalsSchema []byte

var compiled = mustCompile()

func mustCompile() *jsonschema.Schema {
	var doc any
	if err := json.Unmarshal(FileVitalsSchema, &doc); err != nil {
		panic(fmt.Sprintf("schema not valid json: %v", err))
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("filevitals.schema.json", doc); err != nil {
		panic(err)
	}
	s, err := c.Compile("filevitals.schema.json")
	if err != nil {
		panic(err)
	}
	return s
}

// validateJSON validates a raw JSON document against the FileVitals schema.
func validateJSON(b []byte) error {
	var inst any
	if err := json.Unmarshal(b, &inst); err != nil {
		return fmt.Errorf("invalid json: %w", err)
	}
	return compiled.Validate(inst)
}

// ValidateFileVitals marshals fv and validates it against the embedded schema.
func ValidateFileVitals(fv model.FileVitals) error {
	b, err := json.Marshal(fv)
	if err != nil {
		return err
	}
	return validateJSON(b)
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/schema/...`
Expected: PASS (fixtures validate; bad output rejected).

- [ ] **Step 6: Commit**

```bash
git add internal/schema
git commit -m "feat(schema): FileVitals JSON Schema + test gate over fixtures"
```

---

### Task 11: Minimal CLI entrypoint

**Files:**
- Create: `cmd/tfq/main.go`
- Test: `cmd/tfq/main_test.go`

**Interfaces:**
- Consumes: `engine.Inspect`.
- Produces: a binary `tfq` supporting `tfq inspect <file>` → prints indented `FileVitals` JSON to stdout; unknown/missing args → usage on stderr, exit 2; I/O error → message on stderr, exit 1.
- This is a deliberately thin surface; full vocabulary is out of scope (Phase 4).

- [ ] **Step 1: Write the failing test**

```go
// cmd/tfq/main_test.go
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRunInspect(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "n.md")
	if err := os.WriteFile(f, []byte("---\ntitle: T\n---\n# H\n#tag\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, code := run([]string{"inspect", f})
	if code != 0 {
		t.Fatalf("exit %d, out=%s", code, out)
	}
	var fv map[string]any
	if err := json.Unmarshal([]byte(out), &fv); err != nil {
		t.Fatalf("output not json: %v\n%s", err, out)
	}
	if fv["format"] != "markdown" {
		t.Errorf("format = %v", fv["format"])
	}
}

func TestRunUsage(t *testing.T) {
	if _, code := run([]string{}); code != 2 {
		t.Errorf("expected exit 2 for no args, got %d", code)
	}
	if _, code := run([]string{"bogus"}); code != 2 {
		t.Errorf("expected exit 2 for unknown subcommand, got %d", code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/tfq/...`
Expected: FAIL (`run` undefined).

- [ ] **Step 3: Write the implementation**

```go
// cmd/tfq/main.go
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"tfq/internal/engine"
)

// run returns (stdoutText, exitCode). Kept pure for testing; main wires it to os.
func run(args []string) (string, int) {
	if len(args) < 1 {
		return usage(), 2
	}
	switch args[0] {
	case "inspect":
		if len(args) != 2 {
			return usage(), 2
		}
		fv, err := engine.Inspect(args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, "tfq: "+err.Error())
			return "", 1
		}
		b, err := json.MarshalIndent(fv, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, "tfq: "+err.Error())
			return "", 1
		}
		return string(b), 0
	default:
		return usage(), 2
	}
}

func usage() string {
	return "usage: tfq inspect <file>"
}

func main() {
	out, code := run(os.Args[1:])
	if out != "" {
		if code == 0 {
			fmt.Println(out)
		} else {
			fmt.Fprintln(os.Stderr, out)
		}
	}
	os.Exit(code)
}
```

- [ ] **Step 4: Run test, build, and smoke-run**

Run: `go test ./cmd/tfq/...`
Expected: PASS.

Run: `go build -o tfq ./cmd/tfq && ./tfq inspect internal/engine/testdata/note.md | head -5`
Expected: indented JSON beginning with `{` and a `"path"` field.

- [ ] **Step 5: Final full-suite run and commit**

Run: `go vet ./... && go test ./...`
Expected: all packages PASS.

```bash
git add cmd/tfq
git commit -m "feat(cmd): minimal tfq inspect entrypoint"
```

---

## Execution Notes

- **Offline module fetch:** if `go get` cannot reach the network, pre-populate the module cache (`GOMODCACHE`) or set `GOFLAGS=-mod=mod` with a local proxy, then re-run. Both deps are pure-Go and small. Do not substitute hand-rolled YAML/JSON-Schema code — the spec mandates real JSON Schema validation as the test gate.
- **RE2 discipline:** if any pattern needs lookahead/backreference, redesign it with a Go post-processing step instead (as Task 6/7 do with overlap tracking). This preserves ripgrep compatibility for the later corpus path.
- **Determinism:** extractor output order is position-sorted (links) or natural scan order (others); keep it stable so schema/golden tests don't flake.

## Self-Review

- **Spec coverage:** multi-format registry (Task 8) ✓; RE2 extractors for frontmatter (4), headings (5), markers incl. angle brackets (6), links incl. org/obsidian/markdown/autolink/bare (7) ✓; `FileVitals` contract (2) ✓; comprehensive query-by-file JSON (9, 11) ✓; predefined output schema + validation-in-tests (10) ✓; liberal/never-fail extraction (all extractor tasks) ✓; no index/semantic/full-CUE (nothing introduces them) ✓; vocabulary deferred to a thin `inspect` only (11) ✓.
- **Placeholders:** none — every step has runnable code and an expected result.
- **Type consistency:** `model.*` names, `Frontmatter/Headings/Markers/Links` signatures, `FormatFor`, `InspectContent/Inspect`, `ValidateFileVitals`, and `run` are used identically across the tasks that produce and consume them.
