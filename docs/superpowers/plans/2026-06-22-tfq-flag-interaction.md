# tfq flag-interaction switchover — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace tfq's flat-verb CLI with a grep-like flag model (`tfq [OPTIONS] [SELECTOR...]`), human output by default and `--json` universal, without changing any engine output JSON shape.

**Architecture:** `cmd/tfq` stays a thin parse→dispatch→format layer over the unchanged `internal/*` engine. New: a hand-rolled flag parser (`parse.go`), human formatters (`format.go`), and a pure root resolver (`internal/rootdir`). Small additive engine changes: multi-tag/status/ignore-case search, a tag index, and ambiguous-write detection.

**Tech Stack:** Go 1.25, stdlib only (no CLI framework), ripgrep for search, `santhosh-tekuri/jsonschema/v6` for the test-only output gate.

## Global Constraints

- Go 1.25; one static binary; **no new third-party dependency**; parser stays hand-rolled.
- Engine output JSON shapes (`FileVitals`, `Hit`, `ListItem`, `Record`, `Edge`, `WriteResult`, `Report`) are **unchanged** so every `internal/schema/*_test.go` gate stays valid.
- TDD: failing test → confirm RED → minimal impl → GREEN → commit. One commit per task.
- RE2-only, no search index, liberal extraction, body+key-order-preserving writes, `internal/layout` config seam — all preserved.
- Exit codes: `0` success, `1` runtime error / `--validate` not OK, `2` usage error.
- After every task: `go vet ./... && go test ./...` must be green (engine tasks patch the soon-to-be-deleted old `cmd/tfq` call sites to stay green).
- Reference: design spec `docs/superpowers/specs/2026-06-22-tfq-flag-interaction-design.md`.

---

### Task 1: `internal/rootdir` — collection root resolution

**Files:**
- Create: `internal/rootdir/rootdir.go`
- Test: `internal/rootdir/rootdir_test.go`

**Interfaces:**
- Produces: `rootdir.Resolve(explicit, env, startDir string) (string, error)`.

- [ ] **Step 1: Write the failing test**

```go
package rootdir

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveExplicitWins(t *testing.T) {
	got, err := Resolve("/x/explicit", "/x/env", "/x/cwd")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/x/explicit" {
		t.Errorf("got %q, want /x/explicit", got)
	}
}

func TestResolveEnvWhenNoExplicit(t *testing.T) {
	got, _ := Resolve("", "/x/env", "/x/cwd")
	if got != "/x/env" {
		t.Errorf("got %q, want /x/env", got)
	}
}

func TestResolveAncestorMarker(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".tfq.cue"), []byte("status: string\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got, _ := Resolve("", "", sub)
	if got != root {
		t.Errorf("got %q, want %q", got, root)
	}
}

func TestResolveFallsBackToCwd(t *testing.T) {
	dir := t.TempDir()
	got, _ := Resolve("", "", dir)
	if got != dir {
		t.Errorf("got %q, want %q (cwd fallback)", got, dir)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/rootdir/`
Expected: FAIL — `undefined: Resolve`.

- [ ] **Step 3: Write minimal implementation**

```go
// Package rootdir resolves a tfq collection root from explicit flag, env, an
// ancestor marker, or the working directory.
package rootdir

import (
	"os"
	"path/filepath"
)

// markers are files/dirs whose presence marks a collection root.
var markers = []string{".tfq.cue", ".tfq.yaml", ".tfq"}

// Resolve picks the collection root: explicit (--root), then env (TFQ_ROOT),
// then the nearest ancestor of startDir containing a marker, then startDir.
func Resolve(explicit, env, startDir string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if env != "" {
		return env, nil
	}
	dir := startDir
	for {
		for _, m := range markers {
			if _, err := os.Stat(filepath.Join(dir, m)); err == nil {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return startDir, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/rootdir/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/rootdir/
git commit -m "feat(rootdir): resolve collection root (flag/env/ancestor/cwd)"
```

---

### Task 2: `graph.Candidates` — all paths a ref could resolve to

**Files:**
- Modify: `internal/graph/graph.go` (add method after `Resolve`)
- Test: `internal/graph/graph_test.go` (append)

**Interfaces:**
- Consumes: existing `baseNoExt`, `stripSeqPrefix`, `fmString`, `g.records`.
- Produces: `(*Graph).Candidates(ref string) []string` — distinct record paths, sorted.

- [ ] **Step 1: Write the failing test** (append to `internal/graph/graph_test.go`)

```go
func TestCandidatesAmbiguous(t *testing.T) {
	recs := []model.FileVitals{
		{Path: "a.md", Frontmatter: map[string]any{"slug": "dup"}},
		{Path: "b.md", Frontmatter: map[string]any{"title": "dup"}},
	}
	g := Build(recs, DefaultOptions())
	got := g.Candidates("dup")
	if len(got) != 2 {
		t.Fatalf("Candidates(dup) = %#v, want 2 matches", got)
	}
}

func TestCandidatesUnique(t *testing.T) {
	recs := []model.FileVitals{
		{Path: "a.md", Frontmatter: map[string]any{"slug": "x"}},
		{Path: "b.md", Frontmatter: map[string]any{"slug": "y"}},
	}
	g := Build(recs, DefaultOptions())
	if got := g.Candidates("x"); len(got) != 1 || got[0] != "a.md" {
		t.Errorf("Candidates(x) = %#v, want [a.md]", got)
	}
	if got := g.Candidates("ghost"); len(got) != 0 {
		t.Errorf("Candidates(ghost) = %#v, want empty", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/graph/ -run TestCandidates`
Expected: FAIL — `g.Candidates undefined`.

- [ ] **Step 3: Write minimal implementation** (add to `internal/graph/graph.go`)

```go
// Candidates returns every distinct record path the ref could resolve to
// (path / basename / seq-stripped basename / id|slug|title). Unlike Resolve
// (first-writer-wins), it surfaces ambiguity so writers can reject it.
func (g *Graph) Candidates(ref string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, r := range g.records {
		base := baseNoExt(r.Path)
		keys := []string{r.Path, base}
		if s := stripSeqPrefix(base); s != base {
			keys = append(keys, s)
		}
		for _, fk := range []string{"id", "slug", "title"} {
			if s, ok := fmString(r.Frontmatter, fk); ok {
				keys = append(keys, s)
			}
		}
		for _, k := range keys {
			if k == ref || k == baseNoExt(ref) {
				if !seen[r.Path] {
					seen[r.Path] = true
					out = append(out, r.Path)
				}
				break
			}
		}
	}
	sort.Strings(out)
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/graph/`
Expected: PASS (all graph tests).

- [ ] **Step 5: Commit**

```bash
git add internal/graph/
git commit -m "feat(graph): Candidates(ref) surfaces all matching record paths"
```

---

### Task 3: `store.Set` — hard error on ambiguous write selector

**Files:**
- Modify: `internal/store/set.go:16-25` (the resolve block in `Set`)
- Test: `internal/store/set_test.go` (append)

**Interfaces:**
- Consumes: `graph.Candidates` (Task 2).
- Produces: `Set` returns an `ambiguous reference %q (matches …)` error when >1 record matches; unchanged otherwise.

- [ ] **Step 1: Write the failing test** (append to `internal/store/set_test.go`)

```go
func TestSetAmbiguousRefIsError(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, dir, "a.md", "---\nslug: dup\nstatus: pending\n---\n# a\n")
	mustWriteFile(t, dir, "b.md", "---\ntitle: dup\nstatus: pending\n---\n# b\n")

	_, err := Set(dir, "dup", map[string]string{"status": "done"}, nil)
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguous-reference error, got %v", err)
	}
}
```

> If `set_test.go` lacks a `mustWriteFile` helper or `strings` import, add:
> ```go
> func mustWriteFile(t *testing.T, dir, name, content string) {
> 	t.Helper()
> 	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
> 		t.Fatal(err)
> 	}
> }
> ```
> and ensure `import ("os"; "path/filepath"; "strings"; "testing")`. Reuse an existing helper if one is already defined.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/ -run TestSetAmbiguous`
Expected: FAIL — `Set` currently resolves first-writer-wins, returns no error.

- [ ] **Step 3: Write minimal implementation** — replace `set.go:22-25`:

```go
	g := graph.Build(recs, graph.DefaultOptions())
	cands := g.Candidates(ref)
	if len(cands) == 0 {
		return WriteResult{}, fmt.Errorf("no record matches %q", ref)
	}
	if len(cands) > 1 {
		return WriteResult{}, fmt.Errorf("ambiguous reference %q (matches %s)", ref, strings.Join(cands, ", "))
	}
	rel := cands[0]
```

(Delete the old `rel, ok := g.Resolve(ref)` / `if !ok` lines. `strings` is already imported in `set.go`.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/`
Expected: PASS (existing `set` tests still green — unique refs unaffected).

- [ ] **Step 5: Commit**

```bash
git add internal/store/
git commit -m "feat(store): Set hard-errors on ambiguous write reference"
```

---

### Task 4: `search.Filters` — status, multi-tag AND, ignore-case

**Files:**
- Modify: `internal/search/search.go` (`Filters` struct, `Search` rg args, `passesFilters`, the filter gate)
- Modify: `internal/search/search_test.go` (update any `Filters{Tag: …}`; add cases)
- Modify: `cmd/tfq/main.go:52` (patch old call site — file is deleted in Task 9)

**Interfaces:**
- Produces: `search.Filters{Type, Status string; Tags []string; IgnoreCase bool}`; `Search` passes `-i` to rg when `IgnoreCase`.

- [ ] **Step 1: Write the failing test** (append to `internal/search/search_test.go`)

```go
func TestSearchIgnoreCase(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("Needle here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hits, _, err := Search(dir, "needle", Filters{IgnoreCase: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("ignore-case search got %d hits, want 1", len(hits))
	}
}

func TestSearchMultiTagAnd(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("---\ntags: [x, y]\n---\nhello\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.md"), []byte("---\ntags: [x]\n---\nhello\n"), 0o644)

	hits, _, _ := Search(dir, "hello", Filters{Tags: []string{"x", "y"}})
	if len(hits) != 1 || hits[0].Path != "a.md" {
		t.Errorf("multi-tag AND got %#v, want only a.md", hits)
	}
}

func TestSearchStatusFilter(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("---\nstatus: done\n---\nhello\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.md"), []byte("---\nstatus: pending\n---\nhello\n"), 0o644)

	hits, _, _ := Search(dir, "hello", Filters{Status: "pending"})
	if len(hits) != 1 || hits[0].Path != "b.md" {
		t.Errorf("status filter got %#v, want only b.md", hits)
	}
}
```

> If `search_test.go` does not already import `os`/`path/filepath`, add them.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/search/`
Expected: FAIL — `Filters` has no `IgnoreCase`/`Status`/`Tags` fields.

- [ ] **Step 3: Write minimal implementation**

Replace the `Filters` struct (`search.go:22-26`):

```go
// Filters narrows hits by frontmatter (AND semantics; empty matches all).
type Filters struct {
	Type       string
	Status     string
	Tags       []string
	IgnoreCase bool
}
```

Replace the rg invocation (`search.go:43`):

```go
	rgArgs := []string{"--json", "--line-number"}
	if f.IgnoreCase {
		rgArgs = append(rgArgs, "-i")
	}
	rgArgs = append(rgArgs, "--", query, root)
	cmd := exec.Command("rg", rgArgs...)
```

Replace the filter gate (`search.go:79`):

```go
		if f.Type != "" || f.Status != "" || len(f.Tags) > 0 {
```

Replace `passesFilters` (`search.go:104-117`):

```go
func passesFilters(v model.FileVitals, f Filters) bool {
	if f.Type != "" {
		t, ok := v.Frontmatter["type"].(string)
		if !ok || t != f.Type {
			return false
		}
	}
	if f.Status != "" {
		s, ok := v.Frontmatter["status"].(string)
		if !ok || s != f.Status {
			return false
		}
	}
	for _, tag := range f.Tags {
		if !hasTag(v, tag) {
			return false
		}
	}
	return true
}
```

Patch the old CLI call site (`cmd/tfq/main.go:52`, throwaway — removed Task 9):

```go
		sf := search.Filters{Type: flags["type"]}
		if t := flags["tag"]; t != "" {
			sf.Tags = []string{t}
		}
		hits, _, serr := search.Search(pos[1], pos[0], sf)
```

Update any existing `search_test.go` cases that build `Filters{Tag: "x"}` → `Filters{Tags: []string{"x"}}`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go vet ./... && go test ./internal/search/ ./cmd/tfq/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/search/ cmd/tfq/main.go
git commit -m "feat(search): status + multi-tag AND + ignore-case filters"
```

---

### Task 5: `query` — multi-tag list, `Summarize`, tag index, tag groups

**Files:**
- Modify: `internal/query/query.go` (`ListFilters`, `List`, add `Summarize`, `recordTags`, `Tags`, `TagGroups`, types)
- Modify: `internal/query/query_test.go` (update `ListFilters{Tag:…}`; add cases)
- Modify: `cmd/tfq/main.go:130` (patch old call site — deleted Task 9)

**Interfaces:**
- Produces:
  - `query.ListFilters{Status, Type string; Tags []string}`
  - `query.Summarize(r model.FileVitals) ListItem`
  - `query.TagCount{Tag string; Count int}`; `query.Tags(root string) ([]TagCount, error)`
  - `query.TagGroup{Tag string; Count int; Records []ListItem}`; `query.TagGroups(root, substr string) ([]TagGroup, error)`

- [ ] **Step 1: Write the failing test** (append to `internal/query/query_test.go`)

```go
func TestListMultiTagAnd(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("---\ntags: [x, y]\n---\n# a\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.md"), []byte("---\ntags: [x]\n---\n# b\n"), 0o644)

	items, err := List(dir, ListFilters{Tags: []string{"x", "y"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Path != "a.md" {
		t.Errorf("multi-tag list got %#v, want only a.md", items)
	}
}

func TestTagsIndexCounts(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("---\ntags: [x, y]\n---\n# a\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.md"), []byte("---\ntags: [x]\n---\n# b\n"), 0o644)

	tags, err := Tags(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 || tags[0].Tag != "x" || tags[0].Count != 2 {
		t.Errorf("tags index got %#v, want x=2 first", tags)
	}
}

func TestTagGroupsFilterAndMembers(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.md"), []byte("---\ntags: [supply-chain]\n---\n# a\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.md"), []byte("---\ntags: [risk]\n---\n# b\n"), 0o644)

	groups, err := TagGroups(dir, "supply")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].Tag != "supply-chain" || len(groups[0].Records) != 1 {
		t.Errorf("tag groups got %#v, want one supply-chain group with 1 record", groups)
	}
}
```

> If `query_test.go` lacks `os`/`path/filepath` imports, add them.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/query/`
Expected: FAIL — `ListFilters` has no `Tags`; `Tags`/`TagGroups` undefined.

- [ ] **Step 3: Write minimal implementation**

Replace `ListFilters` (`query.go:24-29`):

```go
// ListFilters narrows a List (AND semantics; empty matches all).
type ListFilters struct {
	Status string
	Tags   []string
	Type   string
}
```

In `List`, replace the tag check and the append block (`query.go:81-90`):

```go
		if !hasAllTags(tags, f.Tags) {
			continue
		}
		out = append(out, Summarize(r))
```

Add these functions to `query.go` (place near `List`):

```go
func hasAllTags(have, want []string) bool {
	for _, w := range want {
		if !containsStr(have, w) {
			return false
		}
	}
	return true
}

// Summarize builds the compact ListItem view of a record.
func Summarize(r model.FileVitals) ListItem {
	return ListItem{
		Path:   r.Path,
		Title:  titleOf(r),
		Status: fmStr(r.Frontmatter, "status"),
		Type:   fmStr(r.Frontmatter, "type"),
		Tags:   fmTags(r.Frontmatter),
	}
}

// recordTags is the deduped union of frontmatter tags and #hashtag markers.
func recordTags(r model.FileVitals) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, t := range fmTags(r.Frontmatter) {
		add(t)
	}
	for _, m := range r.Markers {
		if m.Kind == model.MarkerHashtag {
			add(m.Value)
		}
	}
	return out
}

// TagCount is one entry of the tag index.
type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// Tags returns the tag index (count per tag) under root, sorted by count desc
// then name asc.
func Tags(root string) ([]TagCount, error) {
	recs, _, err := scan.Collect(root)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, r := range recs {
		for _, t := range recordTags(r) {
			counts[t]++
		}
	}
	out := make([]TagCount, 0, len(counts))
	for t, c := range counts {
		out = append(out, TagCount{Tag: t, Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Tag < out[j].Tag
	})
	return out, nil
}

// TagGroup is a tag plus its member records.
type TagGroup struct {
	Tag     string     `json:"tag"`
	Count   int        `json:"count"`
	Records []ListItem `json:"records"`
}

// TagGroups returns groups for tags whose name contains substr (all tags when
// substr is ""), each with its member records. Sorted by count desc, name asc.
func TagGroups(root, substr string) ([]TagGroup, error) {
	recs, _, err := scan.Collect(root)
	if err != nil {
		return nil, err
	}
	ls := strings.ToLower(substr)
	byTag := map[string][]ListItem{}
	for _, r := range recs {
		item := Summarize(r)
		for _, t := range recordTags(r) {
			if substr == "" || strings.Contains(strings.ToLower(t), ls) {
				byTag[t] = append(byTag[t], item)
			}
		}
	}
	out := make([]TagGroup, 0, len(byTag))
	for t, members := range byTag {
		sort.Slice(members, func(i, j int) bool { return members[i].Path < members[j].Path })
		out = append(out, TagGroup{Tag: t, Count: len(members), Records: members})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Tag < out[j].Tag
	})
	return out, nil
}
```

(`query.go` already imports `sort`, `strings`, `model`, `scan`.)

Patch the old CLI call site (`cmd/tfq/main.go:130`, throwaway — removed Task 9):

```go
		lf := query.ListFilters{Status: flags["status"], Type: flags["type"]}
		if t := flags["tag"]; t != "" {
			lf.Tags = []string{t}
		}
		items, lerr := query.List(pos[0], lf)
```

Update any existing `query_test.go` case using `ListFilters{Tag: "x"}` → `ListFilters{Tags: []string{"x"}}`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go vet ./... && go test ./internal/query/ ./cmd/tfq/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/query/ cmd/tfq/main.go
git commit -m "feat(query): multi-tag list, Summarize, Tags index, TagGroups"
```

---

### Task 6: schema gate for the tag index

**Files:**
- Create: `internal/schema/tags.schema.json`
- Modify: `internal/schema/schema.go` (embed + compile + `ValidateTags`)
- Test: `internal/schema/tags_schema_test.go`

**Interfaces:**
- Produces: `schema.ValidateTags(tags any) error`.

- [ ] **Step 1: Write the failing test**

```go
package schema

import (
	"testing"

	"tfq/internal/query"
)

func TestTagsOutputMatchesSchema(t *testing.T) {
	tags := []query.TagCount{{Tag: "x", Count: 2}, {Tag: "y", Count: 1}}
	if err := ValidateTags(tags); err != nil {
		t.Errorf("valid tags rejected: %v", err)
	}
}

func TestTagsSchemaRejectsBad(t *testing.T) {
	bad := []map[string]any{{"tag": "x"}} // missing count
	if err := ValidateTags(bad); err == nil {
		t.Error("expected schema rejection for missing count")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/schema/ -run TestTags`
Expected: FAIL — `ValidateTags` undefined.

- [ ] **Step 3: Write minimal implementation**

Create `internal/schema/tags.schema.json`:

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://tfq/schemas/tags.json",
  "title": "Tags",
  "type": "array",
  "items": {
    "type": "object",
    "additionalProperties": false,
    "required": ["tag", "count"],
    "properties": {
      "tag": { "type": "string" },
      "count": { "type": "integer" }
    }
  }
}
```

Add to `internal/schema/schema.go` (next to the other embeds/compiled vars/validators):

```go
//go:embed tags.schema.json
var TagsSchema []byte
```
```go
var compiledTags = mustCompileNamed("tags.schema.json", TagsSchema)
```
```go
// ValidateTags validates the tag index. Takes any to avoid an import cycle.
func ValidateTags(tags any) error { return validateAgainst(compiledTags, tags) }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/schema/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/schema/
git commit -m "test(schema): gate the tag index output (ValidateTags)"
```

---

### Task 7: `cmd/tfq/parse.go` — the flag parser

**Files:**
- Create: `cmd/tfq/parse.go`
- Test: `cmd/tfq/parse_test.go`

This coexists with the old `args.go`/`main.go` (no name clashes), so the package still builds and the old tests still pass.

**Interfaces:**
- Produces: `Mode` (enum), `Invocation` struct, `parse(raw []string) (Invocation, error)`, `usageError` (with `usageErr(msg) error`).
- Consumed by Task 9 (`main.go` dispatch).

`Invocation` fields used downstream: `Mode Mode`, `Selector string`, `Root string`, `JSON bool`, `Type string`, `Status string`, `Tags []string`, `Limit int`, `IgnoreCase bool`, `FilesOnly bool`, `Count bool`, `Heading bool`, `Raw bool`, `Frontmatter bool`, `Inbound bool`, `Outbound bool`, `Strict bool`, `Fields map[string]string`.

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"reflect"
	"testing"
)

func TestParseDefaultSearch(t *testing.T) {
	inv, err := parse([]string{"battery", "supply", "chain"})
	if err != nil {
		t.Fatal(err)
	}
	if inv.Mode != ModeSearch || inv.Selector != "battery supply chain" {
		t.Errorf("got mode=%v selector=%q", inv.Mode, inv.Selector)
	}
}

func TestParseFlagsAndSelectorInterleaved(t *testing.T) {
	inv, err := parse([]string{"--tag", "battery", "supply", "chain", "-i"})
	if err != nil {
		t.Fatal(err)
	}
	if inv.Selector != "supply chain" || !inv.IgnoreCase {
		t.Errorf("selector=%q ignoreCase=%v", inv.Selector, inv.IgnoreCase)
	}
	if !reflect.DeepEqual(inv.Tags, []string{"battery"}) {
		t.Errorf("tags=%#v", inv.Tags)
	}
}

func TestParseModeAndAliases(t *testing.T) {
	if inv, _ := parse([]string{"--done", "task-1"}); inv.Mode != ModeSet || inv.Status != "done" || inv.Selector != "task-1" {
		t.Errorf("--done parsed wrong: %#v", inv)
	}
	if inv, _ := parse([]string{"--task", "do-it"}); inv.Mode != ModeNew || inv.Type != "task" {
		t.Errorf("--task parsed wrong: %#v", inv)
	}
	if inv, _ := parse([]string{"--backlinks", "x"}); inv.Mode != ModeLinks || !inv.Inbound {
		t.Errorf("--backlinks parsed wrong: %#v", inv)
	}
}

func TestParseExplicitQueryAndDashDash(t *testing.T) {
	if inv, _ := parse([]string{"-e", "-foo"}); inv.Selector != "-foo" {
		t.Errorf("-e selector=%q", inv.Selector)
	}
	if inv, _ := parse([]string{"--", "-bar"}); inv.Selector != "-bar" {
		t.Errorf("-- selector=%q", inv.Selector)
	}
}

func TestParseRepeatTagsAndFields(t *testing.T) {
	inv, _ := parse([]string{"--new", "x", "--tag", "a", "--tag", "b", "--field", "k=v"})
	if !reflect.DeepEqual(inv.Tags, []string{"a", "b"}) {
		t.Errorf("tags=%#v", inv.Tags)
	}
	if inv.Fields["k"] != "v" {
		t.Errorf("fields=%#v", inv.Fields)
	}
}

func TestParseErrors(t *testing.T) {
	if _, err := parse([]string{"--show", "--links", "x"}); err == nil {
		t.Error("expected error for two modes")
	}
	if _, err := parse([]string{"--bogus"}); err == nil {
		t.Error("expected error for unknown flag")
	}
	if _, err := parse([]string{"--type"}); err == nil {
		t.Error("expected error for missing value")
	}
	if _, err := parse([]string{"--limit", "x"}); err == nil {
		t.Error("expected error for non-integer limit")
	}
}

func TestParseHeadingDefaultTrue(t *testing.T) {
	if inv, _ := parse([]string{"x"}); !inv.Heading {
		t.Error("heading should default true")
	}
	if inv, _ := parse([]string{"x", "--no-heading"}); inv.Heading {
		t.Error("--no-heading should clear heading")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/tfq/ -run TestParse`
Expected: FAIL — `parse`/`ModeSearch` undefined.

- [ ] **Step 3: Write minimal implementation** — create `cmd/tfq/parse.go`:

```go
package main

import (
	"fmt"
	"strconv"
	"strings"
)

// Mode is the primary operation tfq performs. Search is the default.
type Mode int

const (
	ModeSearch Mode = iota
	ModeList
	ModeShow
	ModeLinks
	ModeTags
	ModeNext
	ModeNew
	ModeSet
	ModeValidate
	ModeInspect
	ModeGraph
	ModeVersion
	ModeHelp
)

// Invocation is a fully parsed command line.
type Invocation struct {
	Mode     Mode
	Selector string
	Root     string
	JSON     bool

	Type   string
	Status string
	Tags   []string
	Limit  int

	IgnoreCase bool
	FilesOnly  bool
	Count      bool
	Heading    bool

	Raw         bool
	Frontmatter bool

	Inbound  bool
	Outbound bool

	Strict bool

	Fields map[string]string
}

// usageError marks an error that should exit 2 (vs 1 for runtime errors).
type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }
func usageErr(msg string) error    { return usageError{msg} }

// shortName maps a single-char short flag to its long name ("" if unknown).
func shortName(s string) string {
	switch s {
	case "i":
		return "ignore-case"
	case "l":
		return "files-with-matches"
	case "c":
		return "count"
	case "e":
		return "query"
	default:
		return ""
	}
}

// parse turns raw args into an Invocation. Non-flag tokens (and -e/--query
// values) join into the selector; -- stops flag parsing; exactly one primary
// mode flag is allowed.
func parse(raw []string) (Invocation, error) {
	inv := Invocation{Mode: ModeSearch, Heading: true, Fields: map[string]string{}}
	var sel []string
	modeFlag := "" // the mode flag already chosen (for the "one mode" error)

	setMode := func(m Mode, name string) error {
		if modeFlag != "" {
			return usageErr(fmt.Sprintf("only one mode allowed (got --%s and --%s)", modeFlag, name))
		}
		inv.Mode = m
		modeFlag = name
		return nil
	}

	i := 0
	for i < len(raw) {
		a := raw[i]
		i++

		if a == "--" {
			sel = append(sel, raw[i:]...)
			break
		}
		if a == "" || a == "-" || a[0] != '-' {
			sel = append(sel, a)
			continue
		}

		var name, val string
		hasVal := false
		if strings.HasPrefix(a, "--") {
			name = a[2:]
			if eq := strings.IndexByte(name, '='); eq >= 0 {
				val, hasVal = name[eq+1:], true
				name = name[:eq]
			}
		} else {
			name = shortName(a[1:])
			if name == "" {
				return inv, usageErr("unknown flag " + a)
			}
		}

		needVal := func() (string, error) {
			if hasVal {
				return val, nil
			}
			if i >= len(raw) {
				return "", usageErr("flag --" + name + " needs a value")
			}
			v := raw[i]
			i++
			return v, nil
		}

		switch name {
		// primary modes
		case "search":
			if err := setMode(ModeSearch, name); err != nil {
				return inv, err
			}
		case "list":
			if err := setMode(ModeList, name); err != nil {
				return inv, err
			}
		case "show":
			if err := setMode(ModeShow, name); err != nil {
				return inv, err
			}
		case "links":
			if err := setMode(ModeLinks, name); err != nil {
				return inv, err
			}
		case "tags":
			if err := setMode(ModeTags, name); err != nil {
				return inv, err
			}
		case "next":
			if err := setMode(ModeNext, name); err != nil {
				return inv, err
			}
		case "new":
			if err := setMode(ModeNew, name); err != nil {
				return inv, err
			}
		case "set":
			if err := setMode(ModeSet, name); err != nil {
				return inv, err
			}
		case "validate":
			if err := setMode(ModeValidate, name); err != nil {
				return inv, err
			}
		case "inspect":
			if err := setMode(ModeInspect, name); err != nil {
				return inv, err
			}
		case "graph":
			if err := setMode(ModeGraph, name); err != nil {
				return inv, err
			}
		case "version":
			if err := setMode(ModeVersion, name); err != nil {
				return inv, err
			}
		case "help":
			if err := setMode(ModeHelp, name); err != nil {
				return inv, err
			}
		// mode aliases
		case "done":
			if err := setMode(ModeSet, name); err != nil {
				return inv, err
			}
			inv.Status = "done"
		case "task":
			if err := setMode(ModeNew, name); err != nil {
				return inv, err
			}
			inv.Type = "task"
		case "backlinks":
			if err := setMode(ModeLinks, name); err != nil {
				return inv, err
			}
			inv.Inbound = true
		case "outlinks", "forward-links":
			if err := setMode(ModeLinks, name); err != nil {
				return inv, err
			}
			inv.Outbound = true
		// universal
		case "json":
			inv.JSON = true
		case "root":
			v, err := needVal()
			if err != nil {
				return inv, err
			}
			inv.Root = v
		case "query":
			v, err := needVal()
			if err != nil {
				return inv, err
			}
			sel = append(sel, v)
		// filters
		case "type":
			v, err := needVal()
			if err != nil {
				return inv, err
			}
			inv.Type = v
		case "status":
			v, err := needVal()
			if err != nil {
				return inv, err
			}
			inv.Status = v
		case "tag":
			v, err := needVal()
			if err != nil {
				return inv, err
			}
			inv.Tags = append(inv.Tags, v)
		case "limit":
			v, err := needVal()
			if err != nil {
				return inv, err
			}
			n, cerr := strconv.Atoi(v)
			if cerr != nil {
				return inv, usageErr("--limit needs an integer, got " + v)
			}
			inv.Limit = n
		case "field":
			v, err := needVal()
			if err != nil {
				return inv, err
			}
			eq := strings.IndexByte(v, '=')
			if eq < 0 {
				return inv, usageErr("--field needs k=v, got " + v)
			}
			inv.Fields[v[:eq]] = v[eq+1:]
		// search output
		case "ignore-case":
			inv.IgnoreCase = true
		case "files-with-matches":
			inv.FilesOnly = true
		case "count":
			inv.Count = true
		case "heading":
			inv.Heading = true
		case "no-heading":
			inv.Heading = false
		// show
		case "raw":
			inv.Raw = true
		case "frontmatter":
			inv.Frontmatter = true
		// links
		case "inbound":
			inv.Inbound = true
		case "outbound":
			inv.Outbound = true
		// validate
		case "strict":
			inv.Strict = true
		default:
			return inv, usageErr("unknown flag --" + name)
		}
	}

	inv.Selector = strings.Join(sel, " ")
	return inv, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go vet ./... && go test ./cmd/tfq/`
Expected: PASS (new parser tests + old tests still green).

- [ ] **Step 5: Commit**

```bash
git add cmd/tfq/parse.go cmd/tfq/parse_test.go
git commit -m "feat(cmd): flag parser -> Invocation (modes, aliases, selector)"
```

---

### Task 8: `cmd/tfq/format.go` — human output formatters

**Files:**
- Create: `cmd/tfq/format.go`
- Test: `cmd/tfq/format_test.go`

Pure functions over engine types (`search.Hit`, `query.ListItem`/`TagCount`/`TagGroup`/`Record`, `graph.Edge`, `store.WriteResult`, `validate.Report`, `model.FileVitals`). They coexist with old `main.go` (which keeps `mustJSON`/`usage`/`errln`); do **not** redefine those here.

**Interfaces:**
- Produces: `summaryLine`, `formatList`, `formatHits`, `filesOf`, `fileCount`+`countsOf`, `formatCounts`, `formatTagsIndex`, `formatTagGroups`, `formatLinks`, `linkedPaths`, `formatRecord`, `formatFrontmatterBlock`, `formatWrite`, `formatReport`, `formatEdges`, `formatInspect`.

- [ ] **Step 1: Write the failing test** — create `cmd/tfq/format_test.go`:

```go
package main

import (
	"strings"
	"testing"

	"tfq/internal/graph"
	"tfq/internal/query"
	"tfq/internal/search"
)

func TestFormatHitsHeading(t *testing.T) {
	hits := []search.Hit{
		{Path: "a.md", Line: 12, Text: "model battery"},
		{Path: "a.md", Line: 37, Text: "supply risk"},
		{Path: "b.md", Line: 8, Text: "cathode"},
	}
	out := formatHits(hits, true)
	if !strings.Contains(out, "a.md\n12: model battery\n37: supply risk") {
		t.Errorf("heading output wrong:\n%s", out)
	}
	flat := formatHits(hits, false)
	if !strings.Contains(flat, "a.md:12:model battery") {
		t.Errorf("no-heading output wrong:\n%s", flat)
	}
}

func TestFilesAndCounts(t *testing.T) {
	hits := []search.Hit{{Path: "a.md", Line: 1}, {Path: "a.md", Line: 2}, {Path: "b.md", Line: 1}}
	if got := filesOf(hits); len(got) != 2 || got[0] != "a.md" {
		t.Errorf("filesOf = %#v", got)
	}
	c := countsOf(hits)
	if len(c) != 2 || c[0].Path != "a.md" || c[0].Count != 2 {
		t.Errorf("countsOf = %#v", c)
	}
}

func TestFormatListBlock(t *testing.T) {
	out := formatList([]query.ListItem{{Path: "x.md", Type: "task", Status: "pending", Tags: []string{"a"}, Title: "Do X"}})
	if !strings.Contains(out, "x.md  task pending #a") || !strings.Contains(out, "title: Do X") {
		t.Errorf("list block wrong:\n%s", out)
	}
}

func TestFormatLinksBothDirections(t *testing.T) {
	out := formatLinks("a.md",
		[]graph.Edge{{From: "a.md", Kind: "wiki", Raw: "b", To: "b.md"}},
		[]string{"c.md"}, true, true)
	if !strings.Contains(out, "# outbound links") || !strings.Contains(out, "==> b.md") {
		t.Errorf("missing outbound:\n%s", out)
	}
	if !strings.Contains(out, "# inbound links") || !strings.Contains(out, "<== c.md") {
		t.Errorf("missing inbound:\n%s", out)
	}
}

func TestFormatTagsIndex(t *testing.T) {
	out := formatTagsIndex([]query.TagCount{{Tag: "battery", Count: 42}})
	if !strings.Contains(out, "battery") || !strings.Contains(out, "42") {
		t.Errorf("tags index wrong:\n%s", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/tfq/ -run 'TestFormat|TestFiles'`
Expected: FAIL — formatters undefined.

- [ ] **Step 3: Write minimal implementation** — create `cmd/tfq/format.go`:

```go
package main

import (
	"fmt"
	"sort"
	"strings"

	"tfq/internal/graph"
	"tfq/internal/model"
	"tfq/internal/query"
	"tfq/internal/search"
	"tfq/internal/store"
	"tfq/internal/validate"
)

// summaryLine renders "path  <type> <status> #tag…" (omitting empty parts).
func summaryLine(path, typ, status string, tags []string) string {
	meta := []string{}
	if typ != "" {
		meta = append(meta, typ)
	}
	if status != "" {
		meta = append(meta, status)
	}
	for _, t := range tags {
		meta = append(meta, "#"+t)
	}
	if len(meta) == 0 {
		return path
	}
	return path + "  " + strings.Join(meta, " ")
}

func formatList(items []query.ListItem) string {
	blocks := make([]string, 0, len(items))
	for _, it := range items {
		b := summaryLine(it.Path, it.Type, it.Status, it.Tags)
		if it.Title != "" {
			b += "\n  title: " + it.Title
		}
		blocks = append(blocks, b)
	}
	return strings.Join(blocks, "\n\n")
}

func formatHits(hits []search.Hit, heading bool) string {
	if len(hits) == 0 {
		return ""
	}
	if !heading {
		lines := make([]string, len(hits))
		for i, h := range hits {
			lines[i] = fmt.Sprintf("%s:%d:%s", h.Path, h.Line, h.Text)
		}
		return strings.Join(lines, "\n")
	}
	var blocks []string
	cur := ""
	var curLines []string
	flush := func() {
		if cur != "" {
			blocks = append(blocks, cur+"\n"+strings.Join(curLines, "\n"))
		}
	}
	for _, h := range hits {
		if h.Path != cur {
			flush()
			cur, curLines = h.Path, nil
		}
		curLines = append(curLines, fmt.Sprintf("%d: %s", h.Line, h.Text))
	}
	flush()
	return strings.Join(blocks, "\n\n")
}

func filesOf(hits []search.Hit) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, h := range hits {
		if !seen[h.Path] {
			seen[h.Path] = true
			out = append(out, h.Path)
		}
	}
	return out
}

type fileCount struct {
	Path  string `json:"path"`
	Count int    `json:"count"`
}

func countsOf(hits []search.Hit) []fileCount {
	order := []string{}
	m := map[string]int{}
	for _, h := range hits {
		if _, ok := m[h.Path]; !ok {
			order = append(order, h.Path)
		}
		m[h.Path]++
	}
	out := make([]fileCount, len(order))
	for i, p := range order {
		out[i] = fileCount{Path: p, Count: m[p]}
	}
	return out
}

func formatCounts(counts []fileCount) string {
	lines := make([]string, len(counts))
	for i, c := range counts {
		lines[i] = fmt.Sprintf("%s:%d", c.Path, c.Count)
	}
	return strings.Join(lines, "\n")
}

func formatTagsIndex(tags []query.TagCount) string {
	if len(tags) == 0 {
		return ""
	}
	w := 0
	for _, t := range tags {
		if len(t.Tag) > w {
			w = len(t.Tag)
		}
	}
	lines := make([]string, len(tags))
	for i, t := range tags {
		lines[i] = fmt.Sprintf("  %-*s  %d", w, t.Tag, t.Count)
	}
	return "# tags\n" + strings.Join(lines, "\n")
}

func formatTagGroups(groups []query.TagGroup) string {
	blocks := make([]string, 0, len(groups))
	for _, g := range groups {
		var b strings.Builder
		fmt.Fprintf(&b, "# %s  %d", g.Tag, g.Count)
		for _, r := range g.Records {
			b.WriteString("\n  ==> " + summaryLine(r.Path, r.Type, r.Status, nil))
		}
		blocks = append(blocks, b.String())
	}
	return strings.Join(blocks, "\n\n")
}

func formatLinks(path string, out []graph.Edge, in []string, showOut, showIn bool) string {
	var b strings.Builder
	b.WriteString(path)
	if showOut {
		b.WriteString("\n\n# outbound links")
		if len(out) == 0 {
			b.WriteString("\n  (none)")
		}
		for _, e := range out {
			to := e.To
			if to == "" {
				to = e.Raw + " (unresolved)"
			}
			b.WriteString("\n  ==> " + to + "\n      " + e.Kind + " " + e.Raw)
		}
	}
	if showIn {
		b.WriteString("\n\n# inbound links")
		if len(in) == 0 {
			b.WriteString("\n  (none)")
		}
		for _, p := range in {
			b.WriteString("\n  <== " + p)
		}
	}
	return b.String()
}

func linkedPaths(out []graph.Edge, in []string, showOut, showIn bool) []string {
	seen := map[string]bool{}
	res := []string{}
	add := func(p string) {
		if p != "" && !seen[p] {
			seen[p] = true
			res = append(res, p)
		}
	}
	if showOut {
		for _, e := range out {
			add(e.To)
		}
	}
	if showIn {
		for _, p := range in {
			add(p)
		}
	}
	sort.Strings(res)
	return res
}

func formatRecord(rec query.Record) string {
	out := rec.Path + "\n" + formatFrontmatterBlock(rec.Frontmatter) + "\n\n" + rec.Body
	return strings.TrimRight(out, "\n")
}

func formatFrontmatterBlock(fm map[string]any) string {
	keys := make([]string, 0, len(fm))
	for k := range fm {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, k := range keys {
		lines = append(lines, k+": "+fmScalar(fm[k]))
	}
	return strings.Join(lines, "\n")
}

func fmScalar(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		parts := make([]string, len(t))
		for i, e := range t {
			parts[i] = fmt.Sprintf("%v", e)
		}
		return strings.Join(parts, ", ")
	default:
		return fmt.Sprintf("%v", t)
	}
}

func formatWrite(res store.WriteResult) string {
	return res.Action + " " + res.Path
}

func formatReport(rep validate.Report) string {
	var b strings.Builder
	if rep.OK {
		b.WriteString("validate: OK")
	} else {
		b.WriteString("validate: FAILED")
	}
	for _, f := range rep.Findings {
		loc := f.Path
		if f.Field != "" {
			loc += " [" + f.Field + "]"
		}
		b.WriteString(fmt.Sprintf("\n  %s: %s (%s)", loc, f.Message, f.Severity))
	}
	return b.String()
}

func formatEdges(edges []graph.Edge) string {
	if len(edges) == 0 {
		return ""
	}
	lines := make([]string, len(edges))
	for i, e := range edges {
		to := e.To
		if to == "" {
			to = e.Raw + " (unresolved)"
		}
		lines[i] = fmt.Sprintf("%s --%s--> %s", e.From, e.Kind, to)
	}
	return strings.Join(lines, "\n")
}

func formatInspect(fv model.FileVitals) string {
	return fmt.Sprintf("%s\n  format: %s\n  headings: %d\n  links: %d\n  markers: %d",
		fv.Path, fv.Format, len(fv.Headings), len(fv.Links), len(fv.Markers))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go vet ./... && go test ./cmd/tfq/`
Expected: PASS (formatters + parser + old tests; formatters are unused by production code so far — Go permits unused package-level funcs).

- [ ] **Step 5: Commit**

```bash
git add cmd/tfq/format.go cmd/tfq/format_test.go
git commit -m "feat(cmd): human output formatters for every mode"
```

---

### Task 9: wire it together — dispatch, root, delete the verbs

**Files:**
- Create: `cmd/tfq/root.go`
- Rewrite: `cmd/tfq/main.go` (`run` body + `usage` text; keep `mustJSON`, `errln`, `buildGraph`, `main`, `version`)
- Delete: `cmd/tfq/args.go`, `cmd/tfq/args_test.go`
- Rewrite: `cmd/tfq/main_test.go`

**Interfaces:**
- Consumes everything from Tasks 1,4,5,7,8.
- Produces: the final CLI. `run(args []string) (string, int)`.

- [ ] **Step 1: Write the failing test** — replace `cmd/tfq/main_test.go` entirely:

```go
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustWrite(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
func contains(s, sub string) bool { return strings.Contains(s, sub) }

func TestRunHelpAndVersion(t *testing.T) {
	if out, code := run([]string{}); code != 0 || !contains(out, "usage") {
		t.Errorf("bare tfq should print help, got code=%d", code)
	}
	if out, code := run([]string{"--version"}); code != 0 || out != version {
		t.Errorf("--version = %q code=%d", out, code)
	}
	if _, code := run([]string{"--bogus"}); code != 2 {
		t.Errorf("unknown flag should exit 2, got %d", code)
	}
	if _, code := run([]string{"--show", "--links", "x"}); code != 2 {
		t.Errorf("two modes should exit 2, got %d", code)
	}
}

func TestRunSearchHumanAndJSON(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "a.md", "needle here\n")

	out, code := run([]string{"--root", dir, "needle"})
	if code != 0 || !contains(out, "a.md") || !contains(out, "1: needle here") {
		t.Errorf("human search wrong: code=%d\n%s", code, out)
	}

	out, code = run([]string{"--root", dir, "needle", "--json"})
	if code != 0 {
		t.Fatalf("json search exit %d: %s", code, out)
	}
	var hits []map[string]any
	if err := json.Unmarshal([]byte(out), &hits); err != nil || len(hits) != 1 {
		t.Errorf("json search wrong: %v\n%s", err, out)
	}

	out, _ = run([]string{"--root", dir, "needle", "-l"})
	if strings.TrimSpace(out) != "a.md" {
		t.Errorf("-l output = %q", out)
	}
}

func TestRunListFolding(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "a.md", "---\nstatus: pending\n---\n# a\n")
	mustWrite(t, dir, "b.md", "---\nstatus: done\n---\n# b\n")

	// empty selector + filter => list behavior
	out, code := run([]string{"--root", dir, "--status", "pending"})
	if code != 0 || !contains(out, "a.md") || contains(out, "b.md") {
		t.Errorf("empty-selector list wrong: %s", out)
	}
	// explicit --list mode
	out, _ = run([]string{"--root", dir, "--list"})
	if !contains(out, "a.md") || !contains(out, "b.md") {
		t.Errorf("--list wrong: %s", out)
	}
}

func TestRunTagsMode(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "a.md", "---\ntags: [battery, supply-chain]\n---\n# a\n")
	mustWrite(t, dir, "b.md", "---\ntags: [battery]\n---\n# b\n")

	out, code := run([]string{"--root", dir, "--tags"})
	if code != 0 || !contains(out, "battery") {
		t.Errorf("tags index wrong: %s", out)
	}
	out, _ = run([]string{"--root", dir, "--tags", "supply"})
	if !contains(out, "supply-chain") || !contains(out, "a.md") {
		t.Errorf("tags search wrong: %s", out)
	}
}

func TestRunLinksBothDirections(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "a.md", "---\nslug: a\n---\nsee [[b]]\n")
	mustWrite(t, dir, "b.md", "---\nslug: b\n---\nlink [[a]]\n")

	out, code := run([]string{"--root", dir, "--links", "a"})
	if code != 0 || !contains(out, "outbound") || !contains(out, "inbound") {
		t.Errorf("links both dirs wrong: %s", out)
	}
	out, _ = run([]string{"--root", dir, "--backlinks", "a"})
	if contains(out, "outbound") {
		t.Errorf("--backlinks should be inbound only: %s", out)
	}
}

func TestRunWriteWorkflow(t *testing.T) {
	dir := t.TempDir()

	// create two tasks
	if _, code := run([]string{"--root", dir, "--new", "first", "--type", "task"}); code != 0 {
		t.Fatal("new first failed")
	}
	if _, code := run([]string{"--root", dir, "--new", "second", "--type", "task"}); code != 0 {
		t.Fatal("new second failed")
	}
	// second depends on first
	if _, code := run([]string{"--root", dir, "--set", "second", "--field", "dependencies=first"}); code != 0 {
		t.Fatal("set dep failed")
	}
	// next gates "second" until "first" is done; "first" is ready
	out, _ := run([]string{"--root", dir, "--next"})
	if !contains(out, "first") || contains(out, "second") {
		t.Errorf("next gating wrong: %s", out)
	}
	// complete first via --done
	if _, code := run([]string{"--root", dir, "--done", "first"}); code != 0 {
		t.Fatal("done failed")
	}
	// now second is ready
	out, _ = run([]string{"--root", dir, "--next"})
	if !contains(out, "second") {
		t.Errorf("second should be ready: %s", out)
	}
	// show --raw prints the body
	out, code := run([]string{"--root", dir, "--show", "second", "--raw"})
	if code != 0 || !contains(out, "second") {
		t.Errorf("show --raw wrong: %s", out)
	}
}

func TestRunAmbiguousWriteErrors(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "a.md", "---\nslug: dup\nstatus: pending\n---\n# a\n")
	mustWrite(t, dir, "b.md", "---\ntitle: dup\nstatus: pending\n---\n# b\n")
	if _, code := run([]string{"--root", dir, "--done", "dup"}); code != 1 {
		t.Errorf("ambiguous write should exit 1, got %d", code)
	}
}

func TestRunInspectAndValidate(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, ".tfq.cue", "status: \"pending\" | \"completed\"\n")
	mustWrite(t, dir, "ok.md", "---\nstatus: completed\n---\n# ok\n")

	f := filepath.Join(dir, "ok.md")
	out, code := run([]string{"--inspect", f, "--json"})
	if code != 0 {
		t.Fatalf("inspect exit %d: %s", code, out)
	}
	var fv map[string]any
	if json.Unmarshal([]byte(out), &fv) != nil || fv["format"] != "markdown" {
		t.Errorf("inspect json wrong: %s", out)
	}

	if _, code := run([]string{"--root", dir, "--validate"}); code != 0 {
		t.Errorf("liberal validate should exit 0, got %d", code)
	}
	mustWrite(t, dir, "bad.md", "---\nstatus: nope\n---\n# bad\n")
	if _, code := run([]string{"--root", dir, "--validate", "--strict"}); code != 1 {
		t.Errorf("strict validate over bad record should exit 1, got %d", code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/tfq/`
Expected: FAIL/compile error (old `run` is verb-based; new tests use flags). This drives the rewrite.

- [ ] **Step 3a: Delete the old parser + its tests**

```bash
git rm cmd/tfq/args.go cmd/tfq/args_test.go
```

- [ ] **Step 3b: Create `cmd/tfq/root.go`**

```go
package main

import (
	"os"

	"tfq/internal/rootdir"
)

// resolveRoot picks the collection root from --root, $TFQ_ROOT, an ancestor
// marker, or the working directory.
func resolveRoot(explicit string) (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return rootdir.Resolve(explicit, os.Getenv("TFQ_ROOT"), wd)
}
```

- [ ] **Step 3c: Rewrite `cmd/tfq/main.go`**

Keep `version`, `buildGraph`, `errln`, `mustJSON`, `main`. Replace `run` and `usage` and add the small dispatch helpers + `linksJSON`. Final file:

```go
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"tfq/internal/cueschema"
	"tfq/internal/engine"
	"tfq/internal/graph"
	"tfq/internal/layout"
	"tfq/internal/model"
	"tfq/internal/query"
	"tfq/internal/scan"
	"tfq/internal/search"
	"tfq/internal/store"
	"tfq/internal/validate"
)

// version is the build version (yyyymmdd.<nth-commit-of-day>.<hash>); overridden
// at build time via -ldflags (see Makefile). Defaults to "dev".
var version = "dev"

type linksJSON struct {
	Path     string       `json:"path"`
	Outbound []graph.Edge `json:"outbound,omitempty"`
	Inbound  []string     `json:"inbound,omitempty"`
}

// run returns (stdoutText, exitCode). Pure for testing; main wires it to os.
func run(args []string) (string, int) {
	if len(args) == 0 {
		return usage(), 0
	}
	inv, err := parse(args)
	if err != nil {
		if ue, ok := err.(usageError); ok {
			return "tfq: " + ue.Error() + "\n\n" + usage(), 2
		}
		return errln(err), 1
	}

	switch inv.Mode {
	case ModeHelp:
		return usage(), 0
	case ModeVersion:
		return version, 0
	}

	// --inspect operates on the selector as a literal file path.
	if inv.Mode == ModeInspect {
		if inv.Selector == "" {
			return needSelector("--inspect")
		}
		fv, ierr := engine.Inspect(inv.Selector)
		if ierr != nil {
			return errln(ierr), 1
		}
		if inv.JSON {
			return mustJSON(fv), 0
		}
		return formatInspect(fv), 0
	}

	root, rerr := resolveRoot(inv.Root)
	if rerr != nil {
		return errln(rerr), 1
	}

	switch inv.Mode {
	case ModeSearch:
		if inv.Selector == "" {
			return dispatchList(root, inv)
		}
		hits, _, serr := search.Search(root, inv.Selector, search.Filters{
			Type: inv.Type, Status: inv.Status, Tags: inv.Tags, IgnoreCase: inv.IgnoreCase})
		if serr != nil {
			return errln(serr), 1
		}
		if inv.Limit > 0 && len(hits) > inv.Limit {
			hits = hits[:inv.Limit]
		}
		if inv.FilesOnly {
			files := filesOf(hits)
			if inv.JSON {
				return mustJSON(files), 0
			}
			return strings.Join(files, "\n"), 0
		}
		if inv.Count {
			counts := countsOf(hits)
			if inv.JSON {
				return mustJSON(counts), 0
			}
			return formatCounts(counts), 0
		}
		if inv.JSON {
			return mustJSON(hits), 0
		}
		return formatHits(hits, inv.Heading), 0

	case ModeList:
		return dispatchList(root, inv)

	case ModeShow:
		if inv.Selector == "" {
			return needSelector("--show")
		}
		rec, qerr := query.Read(root, inv.Selector)
		if qerr != nil {
			return errln(qerr), 1
		}
		if inv.JSON {
			return mustJSON(rec), 0
		}
		if inv.Raw {
			return rec.Body, 0
		}
		if inv.Frontmatter {
			return formatFrontmatterBlock(rec.Frontmatter), 0
		}
		return formatRecord(rec), 0

	case ModeLinks:
		if inv.Selector == "" {
			return needSelector("--links")
		}
		g, gerr := buildGraph(root)
		if gerr != nil {
			return errln(gerr), 1
		}
		showOut := inv.Outbound || !inv.Inbound
		showIn := inv.Inbound || !inv.Outbound
		out := g.Forward(inv.Selector)
		in := g.Backlinks(inv.Selector)
		if inv.FilesOnly {
			paths := linkedPaths(out, in, showOut, showIn)
			if inv.JSON {
				return mustJSON(paths), 0
			}
			return strings.Join(paths, "\n"), 0
		}
		if inv.JSON {
			p := linksJSON{Path: inv.Selector}
			if showOut {
				p.Outbound = out
			}
			if showIn {
				p.Inbound = in
			}
			return mustJSON(p), 0
		}
		return formatLinks(inv.Selector, out, in, showOut, showIn), 0

	case ModeTags:
		if inv.Selector == "" {
			tags, terr := query.Tags(root)
			if terr != nil {
				return errln(terr), 1
			}
			if inv.JSON {
				return mustJSON(tags), 0
			}
			return formatTagsIndex(tags), 0
		}
		groups, terr := query.TagGroups(root, inv.Selector)
		if terr != nil {
			return errln(terr), 1
		}
		if inv.JSON {
			return mustJSON(groups), 0
		}
		return formatTagGroups(groups), 0

	case ModeNext:
		g, gerr := buildGraph(root)
		if gerr != nil {
			return errln(gerr), 1
		}
		ready, _ := g.Next(graph.DefaultNextOptions())
		ready = filterReady(ready, inv)
		if inv.Limit > 0 && len(ready) > inv.Limit {
			ready = ready[:inv.Limit]
		}
		if inv.JSON {
			return mustJSON(ready), 0
		}
		items := make([]query.ListItem, len(ready))
		for i, r := range ready {
			items[i] = query.Summarize(r)
		}
		return formatList(items), 0

	case ModeNew:
		if inv.Selector == "" {
			return needSelector("--new")
		}
		tmpl := layout.TemplateNote
		if inv.Type == "task" {
			tmpl = layout.TemplateTask
		}
		fields := map[string]string{}
		for k, v := range inv.Fields {
			fields[k] = v
		}
		if inv.Status != "" {
			fields["status"] = inv.Status
		}
		res, nerr := store.New(root, tmpl, inv.Selector, fields, time.Now(), layout.DefaultConfig())
		if nerr != nil {
			return errln(nerr), 1
		}
		if len(inv.Tags) > 0 {
			if _, serr := store.Set(root, inv.Selector, nil, inv.Tags); serr != nil {
				return errln(serr), 1
			}
		}
		if inv.JSON {
			return mustJSON(res), 0
		}
		return formatWrite(res), 0

	case ModeSet:
		if inv.Selector == "" {
			return needSelector("--set")
		}
		fields := map[string]string{}
		for k, v := range inv.Fields {
			fields[k] = v
		}
		if inv.Status != "" {
			fields["status"] = inv.Status
		}
		res, serr := store.Set(root, inv.Selector, fields, inv.Tags)
		if serr != nil {
			return errln(serr), 1
		}
		if inv.JSON {
			return mustJSON(res), 0
		}
		return formatWrite(res), 0

	case ModeValidate:
		rep, verr := validate.Run(root, inv.Strict)
		if verr != nil {
			return errln(verr), 1
		}
		code := 0
		if !rep.OK {
			code = 1
		}
		if inv.JSON {
			return mustJSON(rep), code
		}
		return formatReport(rep), code

	case ModeGraph:
		g, gerr := buildGraph(root)
		if gerr != nil {
			return errln(gerr), 1
		}
		edges := g.Edges()
		if inv.JSON {
			return mustJSON(edges), 0
		}
		return formatEdges(edges), 0
	}
	return usage(), 2
}

func dispatchList(root string, inv Invocation) (string, int) {
	items, lerr := query.List(root, query.ListFilters{Status: inv.Status, Type: inv.Type, Tags: inv.Tags})
	if lerr != nil {
		return errln(lerr), 1
	}
	if inv.Selector != "" {
		items = filterItems(items, inv.Selector)
	}
	if inv.Limit > 0 && len(items) > inv.Limit {
		items = items[:inv.Limit]
	}
	if inv.JSON {
		return mustJSON(items), 0
	}
	return formatList(items), 0
}

func filterItems(items []query.ListItem, sel string) []query.ListItem {
	s := strings.ToLower(sel)
	out := []query.ListItem{}
	for _, it := range items {
		if strings.Contains(strings.ToLower(it.Path), s) || strings.Contains(strings.ToLower(it.Title), s) {
			out = append(out, it)
		}
	}
	return out
}

func filterReady(ready []model.FileVitals, inv Invocation) []model.FileVitals {
	sel := strings.ToLower(inv.Selector)
	out := []model.FileVitals{}
	for _, r := range ready {
		it := query.Summarize(r)
		if inv.Type != "" && it.Type != inv.Type {
			continue
		}
		if inv.Status != "" && it.Status != inv.Status {
			continue
		}
		if len(inv.Tags) > 0 && !hasAllTags(it.Tags, inv.Tags) {
			continue
		}
		if sel != "" && !strings.Contains(strings.ToLower(it.Path), sel) && !strings.Contains(strings.ToLower(it.Title), sel) {
			continue
		}
		out = append(out, r)
	}
	return out
}

func hasAllTags(have, want []string) bool {
	set := map[string]bool{}
	for _, t := range have {
		set[t] = true
	}
	for _, w := range want {
		if !set[w] {
			return false
		}
	}
	return true
}

func needSelector(mode string) (string, int) {
	return "tfq: " + mode + " requires a selector\n\n" + usage(), 2
}

func buildGraph(dir string) (*graph.Graph, error) {
	recs, _, err := scan.Collect(dir)
	if err != nil {
		return nil, err
	}
	opts := graph.DefaultOptions()
	if path, ok := cueschema.Find(dir); ok {
		if s, lerr := cueschema.Load(path); lerr == nil {
			if efs := s.EdgeFields(); len(efs) > 0 {
				names := make([]string, len(efs))
				for i, e := range efs {
					names[i] = e.Name
				}
				opts = graph.Options{FrontmatterEdgeFields: names}
			}
		}
	}
	return graph.Build(recs, opts), nil
}

func errln(err error) string {
	fmt.Fprintln(os.Stderr, "tfq: "+err.Error())
	return ""
}

func mustJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(b)
}

func usage() string {
	return strings.Join([]string{
		"tfq — query frontmatter'd text records",
		"",
		"usage: tfq [OPTIONS] [SELECTOR...]",
		"",
		"Default (search):",
		"  tfq battery supply        search records",
		"  tfq -i battery            case-insensitive search",
		"  tfq -l battery            matching files only",
		"  tfq --status pending      list records (empty selector)",
		"",
		"Modes (one at a time):",
		"  --list [QUERY]            record summaries",
		"  --show REF                show one record (--raw, --frontmatter)",
		"  --links REF               outbound + inbound links (--inbound/--outbound)",
		"  --tags [QUERY]            tag index / tag search",
		"  --next [QUERY]            ready tasks (deps satisfied)",
		"  --new SLUG                create record (--type, --tag, --status, --field)",
		"  --set REF                 update record (--status, --tag, --field)",
		"  --done REF                mark task done",
		"  --validate                validate collection (--strict)",
		"  --inspect FILE            FileVitals for one file",
		"  --graph                   all resolved edges",
		"  --version  --help",
		"",
		"Aliases: --task (=--new --type task), --backlinks (=--links --inbound),",
		"         --outlinks/--forward-links (=--links --outbound)",
		"",
		"Filters:  --type T   --tag T (repeatable)   --status S   --limit N",
		"Search:   -i/--ignore-case   -l/--files-with-matches   -c/--count   --heading/--no-heading",
		"Output:   --json   --root DIR (else $TFQ_ROOT, ancestor .tfq.*, cwd)   -e/--query PATTERN",
	}, "\n")
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

- [ ] **Step 4: Run tests to verify they pass**

Run: `go vet ./... && go test ./...`
Expected: PASS across all packages.

- [ ] **Step 5: Build and smoke-test the binary**

Run:
```bash
make build
./tfq --help | head -3
T=$(mktemp -d); ./tfq --root "$T" --new demo --type task; ./tfq --root "$T" --next; ./tfq --root "$T" --done demo; ./tfq --root "$T" --list
```
Expected: help prints; `created …/demo.md`; `--next` shows demo, then after `--done` the list shows it `completed`.

- [ ] **Step 6: Commit**

```bash
git add cmd/tfq/
git commit -m "feat(cmd): flag-based dispatch; remove flat verbs"
```

---

### Task 10: documentation switchover + delete VOCABULARY-NEW.md

**Files:**
- Rewrite: `VOCABULARY.md`
- Modify: `HANDOFF.md` (§1 surface, §3 CLI-grammar row, §4 `cmd/tfq` row, §10 remove "human output deferred", §13 quickref)
- Delete: `VOCABULARY-NEW.md`

- [ ] **Step 1: Rewrite `VOCABULARY.md`** to document the flag model: the `tfq [OPTIONS] [SELECTOR...]` grammar; the mode table (default search + `--list/--show/--links/--tags/--next/--new/--set/--done/--validate/--inspect/--graph/--version/--help`) and aliases; filters (`--type/--tag×N/--status/--limit`); search flags (`-i/-l/-c/--heading/--no-heading`); show/links flags; `--json`, `--root`, `TFQ_ROOT`, ancestor resolution; the selector-vs-ref note; exit codes; the **Deferred** section (deep ripgrep flags, `--where/--has/--missing/--before/--after`, `--depth`, JSON-Lines, grouped-member tag search beyond substring filter, etc.); and keep the existing **Path policy** section. Replace the old verb table wholesale.

- [ ] **Step 2: Update `HANDOFF.md`**
  - §1 / §"verb surface": replace the `inspect · read · search · …` line with the flag surface and note "See `VOCABULARY.md`".
  - §3 table: change the **CLI grammar** row from "Flat verbs" to "Flag-based, grep-like (`tfq [OPTIONS] [SELECTOR...]`); default mode search; selector string; no positional dir (root resolved)". Keep the rationale honest (verbs were the first cut; VOCABULARY-NEW.md drove the switch).
  - §4 package map: update the `cmd/tfq` row to "flag parser (`parse.go`) + mode dispatch + human formatters (`format.go`); `internal/rootdir` for root resolution".
  - §10 "Human output format": remove from the deferred list (now shipped); add the genuinely-still-deferred items (deep grep flags, structured predicates) if worth noting.
  - §13 Quickref: rewrite the example commands to the flag form (`./tfq --root ./vault "term" --type note`, `./tfq --root ./vault --links some-note`, `./tfq --root ./vault --next`, `./tfq --root ./vault --new my-idea`, etc.).

- [ ] **Step 3: Delete the proposal**

```bash
git rm VOCABULARY-NEW.md
```

- [ ] **Step 4: Final verification**

Run: `go vet ./... && go test ./... && make build && ./tfq --help`
Expected: all green; help reflects the new model; no references to old verbs remain in docs (`grep -rn "tfq search\|tfq read\|tfq backlinks" VOCABULARY.md HANDOFF.md` returns nothing).

- [ ] **Step 5: Commit**

```bash
git add VOCABULARY.md HANDOFF.md
git rm VOCABULARY-NEW.md
git commit -m "docs: switch VOCABULARY/HANDOFF to flag model; drop VOCABULARY-NEW"
```

---

## Self-Review (completed during planning)

- **Spec coverage:** root resolution (T1), ambiguous-write hard error (T2–T3), search filters incl. ignore-case/multi-tag/status (T4), list-fold + tags mode + Summarize (T5, T9), schema discipline (T6), parser/selector/modes/aliases (T7), human formatters + `--json` (T8–T9), clean verb removal (T9), docs + delete (T10). Every spec §2 in-scope item maps to a task.
- **Deferred items** are documented, not silently dropped (T10 step 1). Note: grouped-member tag *search* is implemented (substring filter + members); only the deeper grep flags and metadata predicates are deferred — consistent with spec §2.
- **Type consistency:** `Invocation` fields in T7 match their uses in T9; `search.Filters`/`query.ListFilters`/`query.Tags`/`query.TagGroups`/`graph.Candidates`/`store.Set` signatures match across producing and consuming tasks; formatters in T8 consume the exact engine types (`search.Hit`, `query.ListItem/TagCount/TagGroup/Record`, `graph.Edge`, `store.WriteResult`, `validate.Report`, `model.FileVitals`).
- **Green-at-every-task:** engine signature changes (T4, T5) patch the soon-deleted old `cmd/tfq/main.go` call sites so the tree compiles and tests pass until T9 removes them.
