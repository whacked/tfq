# tfq Corpus Operations Implementation Plan (Phase 2)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build corpus-level operations over a directory of recognized text files — a typed graph (backlinks, forward links, dependency-aware `next`) built from the Phase 1 extracted edges, plus ripgrep-backed search with frontmatter filters — each with a predefined JSON Schema gated in tests.

**Architecture:** A `scan` package walks a root and produces `[]model.FileVitals` (reusing the Phase 1 engine). A `graph` package indexes those records by multiple keys (path, basename, frontmatter id/slug/title) and resolves edges from body links + configurable frontmatter fields; it answers `Backlinks`, `Forward`, and `Next`. A `search` package shells out to `rg --json` and post-filters hits by frontmatter. New CLI verbs expose each. Output of every mode validates against a JSON Schema in tests.

**Tech Stack:** Go 1.25, ripgrep on PATH (search), `github.com/santhosh-tekuri/jsonschema/v6` (schema gates). Builds on Phase 1 packages: `model`, `extract`, `registry`, `engine`, `schema`.

## Global Constraints

- Builds on the existing `tfq` module; do not modify Phase 1 package public signatures.
- **Liberal multi-key edge resolution:** a target resolves against any node key (path, basename-no-ext, frontmatter `id`/`slug`/`title`). Unresolved → `model.Warning`, never an error.
- **`next` semantics:** a record is a task iff it has a `status` frontmatter field. Dependency field defaults to `dependencies`; a dependency is satisfied iff its resolved target's status ∈ {`completed`, `done`, `cancelled`}; an unresolved dependency blocks and emits a warning. A task is "ready" iff its own status ∉ {`completed`, `done`, `cancelled`} and all dependencies are satisfied.
- **Default frontmatter edge fields:** `dependencies`, `parent`, `supersedes`, `source_notes`, `context`.
- **rg discipline:** invoke `rg --json` and parse the stream; never build shell strings by concatenation (pass args as a slice to `exec.Command`).
- Positions are 1-based. Slices in all outputs are non-nil (`[]`, not `null`).
- Every new interaction mode has a JSON Schema validated against real fixture output in tests. A mode without a passing schema test is not done.

---

### Task 1: Scan a directory into records

**Files:**
- Create: `internal/scan/scan.go`
- Test: `internal/scan/scan_test.go`
- Test fixtures: `internal/scan/testdata/vault/note-a.md`, `internal/scan/testdata/vault/sub/note-b.org`, `internal/scan/testdata/vault/ignore.txt`

**Interfaces:**
- Consumes: `engine.InspectContent`, `registry.FormatFor`, `model.FileVitals`, `model.Warning`.
- Produces: `func Collect(root string) ([]model.FileVitals, []model.Warning, error)`.
  - Walks `root` recursively. Skips any directory entry whose name begins with `.` (e.g. `.git`, `.obsidian`).
  - Includes a file only if `registry.FormatFor(ext) != "text"` (markdown/org today).
  - Each record's `Path` is set **relative to `root`** (slash-separated) so downstream keys are stable.
  - Unreadable files produce a warning and are skipped, not a hard error. Only a failure to walk `root` returns an error.
  - Output sorted by `Path` for determinism.

- [ ] **Step 1: Write fixtures**

```text
<!-- internal/scan/testdata/vault/note-a.md -->
---
slug: note-a
---
# Note A
Links to [[note-b]].
```

```text
# internal/scan/testdata/vault/sub/note-b.org
* Note B :topic:
```

```text
<!-- internal/scan/testdata/vault/ignore.txt -->
this is a plain text file and must be skipped
```

- [ ] **Step 2: Write the failing test**

```go
// internal/scan/scan_test.go
package scan

import "testing"

func TestCollect(t *testing.T) {
	recs, warns, err := Collect("testdata/vault")
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
	if len(recs) != 2 {
		t.Fatalf("expected 2 records (md + org), got %d: %#v", len(recs), recs)
	}
	// sorted by relative path: "note-a.md" before "sub/note-b.org"
	if recs[0].Path != "note-a.md" {
		t.Errorf("rec0 path = %q, want note-a.md", recs[0].Path)
	}
	if recs[1].Path != "sub/note-b.org" {
		t.Errorf("rec1 path = %q, want sub/note-b.org", recs[1].Path)
	}
	if recs[1].Format != "org" {
		t.Errorf("rec1 format = %q, want org", recs[1].Format)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/scan/...`
Expected: FAIL (`Collect` undefined).

- [ ] **Step 4: Write the implementation**

```go
// internal/scan/scan.go
package scan

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"tfq/internal/engine"
	"tfq/internal/model"
	"tfq/internal/registry"
)

// Collect walks root and inspects every file whose extension maps to a known
// document format (not "text"). Paths in the returned records are relative to
// root, slash-separated. Unreadable files become warnings, not errors.
func Collect(root string) ([]model.FileVitals, []model.Warning, error) {
	var recs []model.FileVitals
	var warns []model.Warning

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path != root && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if registry.FormatFor(filepath.Ext(name)) == "text" {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			warns = append(warns, model.Warning{Module: "scan", Message: "cannot read " + path + ": " + rerr.Error()})
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)
		recs = append(recs, engine.InspectContent(rel, string(b)))
		return nil
	})
	if err != nil {
		return nil, warns, err
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].Path < recs[j].Path })
	return recs, warns, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/scan/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/scan
git commit -m "feat(scan): collect a directory into FileVitals records"
```

---

### Task 2: Graph node-key index + edge resolution

**Files:**
- Create: `internal/graph/graph.go`
- Test: `internal/graph/graph_test.go`

**Interfaces:**
- Consumes: `model.FileVitals`, `model.Warning`.
- Produces:
  - `type Edge struct { From, Kind, Raw, To string }` (JSON tags `from,kind,raw,to`; `To` is `""` when dangling)
  - `type Options struct { FrontmatterEdgeFields []string }`
  - `func DefaultOptions() Options` → fields `["dependencies","parent","supersedes","source_notes","context"]`
  - `func Build(records []model.FileVitals, opts Options) *Graph`
  - `func (g *Graph) Resolve(ref string) (string, bool)` — ref by any key → canonical record path
  - `func (g *Graph) Edges() []Edge` — all edges, sorted by `From` then `Raw`
  - `func (g *Graph) Warnings() []model.Warning` — dangling-edge warnings
  - Body-link edges use `Kind` = the link kind (`wiki`, `markdown`, `embed`, `org`, `autolink`, `bare-url`); frontmatter edges use `Kind` = `"fm:" + field`.

**Resolution key rules** (per record at path `P`, vitals `V`):
- `P` itself
- `basename(P)` without extension
- `V.Frontmatter["id"]`, `["slug"]`, `["title"]` when they are strings

**Target normalization** for a raw edge target `R`: try `R` as-is; else `basename(R)` without extension. First key match wins; ties broken by record path order.

- [ ] **Step 1: Write the failing test**

```go
// internal/graph/graph_test.go
package graph

import (
	"testing"

	"tfq/internal/model"
)

func recPath(path string, fm map[string]any, links ...model.Link) model.FileVitals {
	return model.FileVitals{
		Path: path, Format: "markdown",
		Frontmatter: fm, Headings: []model.Heading{},
		Links: links, Markers: []model.Marker{}, Warnings: []model.Warning{},
	}
}

func TestResolveByKeys(t *testing.T) {
	recs := []model.FileVitals{
		recPath("notes/note-a.md", map[string]any{"slug": "alpha"}),
		recPath("tasks/001-review.md", map[string]any{"id": "001"}),
	}
	g := Build(recs, DefaultOptions())

	cases := map[string]string{
		"notes/note-a.md": "notes/note-a.md", // exact path
		"note-a":          "notes/note-a.md", // basename
		"alpha":           "notes/note-a.md", // slug
		"001":             "tasks/001-review.md", // id
		"001-review":      "tasks/001-review.md", // basename
	}
	for ref, want := range cases {
		got, ok := g.Resolve(ref)
		if !ok || got != want {
			t.Errorf("Resolve(%q) = %q ok=%v, want %q", ref, got, ok, want)
		}
	}
	if _, ok := g.Resolve("nonexistent"); ok {
		t.Errorf("expected nonexistent to not resolve")
	}
}

func TestEdgesResolveAndDangle(t *testing.T) {
	a := recPath("a.md", map[string]any{"slug": "a"},
		model.Link{Kind: model.LinkWiki, Target: "b", Line: 1, Col: 1})
	b := recPath("b.md", map[string]any{
		"slug":         "b",
		"dependencies": []any{"a"},
		"parent":       "ghost",
	})
	g := Build([]model.FileVitals{a, b}, DefaultOptions())

	var wikiTo, depTo, parentTo string
	parentSeen := false
	for _, e := range g.Edges() {
		switch {
		case e.From == "a.md" && e.Kind == "wiki":
			wikiTo = e.To
		case e.From == "b.md" && e.Kind == "fm:dependencies":
			depTo = e.To
		case e.From == "b.md" && e.Kind == "fm:parent":
			parentTo = e.To
			parentSeen = true
		}
	}
	if wikiTo != "b.md" {
		t.Errorf("wiki a->b resolved to %q", wikiTo)
	}
	if depTo != "a.md" {
		t.Errorf("dep b->a resolved to %q", depTo)
	}
	if !parentSeen || parentTo != "" {
		t.Errorf("parent ghost should dangle (To==\"\"), seen=%v to=%q", parentSeen, parentTo)
	}
	if len(g.Warnings()) == 0 {
		t.Errorf("expected a dangling-edge warning for parent: ghost")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/graph/...`
Expected: FAIL (`Build` etc undefined).

- [ ] **Step 3: Write the implementation**

```go
// internal/graph/graph.go
package graph

import (
	"path"
	"sort"
	"strings"

	"tfq/internal/model"
)

// Edge is a typed, possibly-dangling reference between records.
type Edge struct {
	From string `json:"from"`
	Kind string `json:"kind"`
	Raw  string `json:"raw"`
	To   string `json:"to"` // "" if unresolved
}

// Options configures which frontmatter fields are treated as edges.
type Options struct {
	FrontmatterEdgeFields []string
}

// DefaultOptions returns the default frontmatter edge fields.
func DefaultOptions() Options {
	return Options{FrontmatterEdgeFields: []string{"dependencies", "parent", "supersedes", "source_notes", "context"}}
}

// Graph is an in-memory typed graph over records.
type Graph struct {
	records []model.FileVitals
	byKey   map[string]string // key -> canonical record path (first writer wins)
	edges   []Edge
	warns   []model.Warning
}

func baseNoExt(p string) string {
	b := path.Base(p)
	if i := strings.LastIndex(b, "."); i > 0 {
		b = b[:i]
	}
	return b
}

func fmString(fm map[string]any, key string) (string, bool) {
	if v, ok := fm[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s, true
		}
	}
	return "", false
}

// Build indexes records by key and resolves all edges.
func Build(records []model.FileVitals, opts Options) *Graph {
	g := &Graph{records: records, byKey: map[string]string{}}

	addKey := func(key, p string) {
		if key == "" {
			return
		}
		if _, exists := g.byKey[key]; !exists {
			g.byKey[key] = p
		}
	}
	for _, r := range records {
		addKey(r.Path, r.Path)
		addKey(baseNoExt(r.Path), r.Path)
		for _, fk := range []string{"id", "slug", "title"} {
			if s, ok := fmString(r.Frontmatter, fk); ok {
				addKey(s, r.Path)
			}
		}
	}

	resolveRaw := func(raw string) string {
		if p, ok := g.byKey[raw]; ok {
			return p
		}
		if p, ok := g.byKey[baseNoExt(raw)]; ok {
			return p
		}
		return ""
	}

	for _, r := range records {
		for _, l := range r.Links {
			to := resolveRaw(l.Target)
			g.edges = append(g.edges, Edge{From: r.Path, Kind: l.Kind, Raw: l.Target, To: to})
			if to == "" {
				g.warns = append(g.warns, model.Warning{Module: "graph", Message: r.Path + ": dangling " + l.Kind + " link -> " + l.Target})
			}
		}
		for _, field := range opts.FrontmatterEdgeFields {
			for _, raw := range edgeValues(r.Frontmatter[field]) {
				to := resolveRaw(raw)
				g.edges = append(g.edges, Edge{From: r.Path, Kind: "fm:" + field, Raw: raw, To: to})
				if to == "" {
					g.warns = append(g.warns, model.Warning{Module: "graph", Message: r.Path + ": dangling " + field + " -> " + raw})
				}
			}
		}
	}
	sort.SliceStable(g.edges, func(i, j int) bool {
		if g.edges[i].From != g.edges[j].From {
			return g.edges[i].From < g.edges[j].From
		}
		return g.edges[i].Raw < g.edges[j].Raw
	})
	return g
}

// edgeValues normalizes a frontmatter value into a list of raw edge targets.
func edgeValues(v any) []string {
	switch t := v.(type) {
	case string:
		if t != "" {
			return []string{t}
		}
	case []any:
		out := []string{}
		for _, e := range t {
			if s, ok := e.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return t
	}
	return nil
}

// Resolve maps a reference (by any key) to a canonical record path.
func (g *Graph) Resolve(ref string) (string, bool) {
	if p, ok := g.byKey[ref]; ok {
		return p, true
	}
	if p, ok := g.byKey[baseNoExt(ref)]; ok {
		return p, true
	}
	return "", false
}

// Edges returns all edges (sorted).
func (g *Graph) Edges() []Edge { return g.edges }

// Warnings returns dangling-edge warnings.
func (g *Graph) Warnings() []model.Warning { return g.warns }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/graph/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/graph/graph.go internal/graph/graph_test.go
git commit -m "feat(graph): multi-key node index + liberal edge resolution"
```

---

### Task 3: Backlinks and forward links

**Files:**
- Modify: `internal/graph/graph.go` (add methods)
- Test: `internal/graph/backlinks_test.go`

**Interfaces:**
- Produces:
  - `func (g *Graph) Backlinks(ref string) []string` — sorted unique source paths whose edges resolve to `ref`; empty slice (non-nil) if none or ref unknown.
  - `func (g *Graph) Forward(ref string) []Edge` — edges whose `From` resolves to `ref`.

- [ ] **Step 1: Write the failing test**

```go
// internal/graph/backlinks_test.go
package graph

import (
	"reflect"
	"testing"

	"tfq/internal/model"
)

func TestBacklinksAndForward(t *testing.T) {
	a := recPath("a.md", map[string]any{"slug": "a"},
		model.Link{Kind: model.LinkWiki, Target: "c", Line: 1, Col: 1})
	b := recPath("b.md", map[string]any{"slug": "b"},
		model.Link{Kind: model.LinkWiki, Target: "c", Line: 1, Col: 1})
	c := recPath("c.md", map[string]any{"slug": "c"})
	g := Build([]model.FileVitals{a, b, c}, DefaultOptions())

	bl := g.Backlinks("c")
	if !reflect.DeepEqual(bl, []string{"a.md", "b.md"}) {
		t.Errorf("Backlinks(c) = %#v, want [a.md b.md]", bl)
	}
	if got := g.Backlinks("a"); len(got) != 0 {
		t.Errorf("Backlinks(a) = %#v, want empty", got)
	}
	fwd := g.Forward("a.md")
	if len(fwd) != 1 || fwd[0].To != "c.md" {
		t.Errorf("Forward(a) = %#v", fwd)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/graph/... -run TestBacklinks`
Expected: FAIL (`Backlinks`/`Forward` undefined).

- [ ] **Step 3: Add the implementation to `internal/graph/graph.go`**

```go
// Backlinks returns sorted unique source paths whose edges resolve to ref.
func (g *Graph) Backlinks(ref string) []string {
	target, ok := g.Resolve(ref)
	if !ok {
		return []string{}
	}
	seen := map[string]bool{}
	out := []string{}
	for _, e := range g.edges {
		if e.To == target && !seen[e.From] {
			seen[e.From] = true
			out = append(out, e.From)
		}
	}
	sort.Strings(out)
	return out
}

// Forward returns edges originating from the record ref resolves to.
func (g *Graph) Forward(ref string) []Edge {
	src, ok := g.Resolve(ref)
	if !ok {
		return []Edge{}
	}
	out := []Edge{}
	for _, e := range g.edges {
		if e.From == src {
			out = append(out, e)
		}
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/graph/... -run TestBacklinks`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/graph/graph.go internal/graph/backlinks_test.go
git commit -m "feat(graph): backlinks and forward-link queries"
```

---

### Task 4: Dependency-aware `next`

**Files:**
- Create: `internal/graph/next.go`
- Test: `internal/graph/next_test.go`

**Interfaces:**
- Produces:
  - `type NextOptions struct { DepField, StatusField string; DoneStatuses []string }`
  - `func DefaultNextOptions() NextOptions` → `DepField:"dependencies"`, `StatusField:"status"`, `DoneStatuses:["completed","done","cancelled"]`
  - `func (g *Graph) Next(o NextOptions) ([]model.FileVitals, []model.Warning)` — records that are tasks (have the status field), are not themselves done, and all of whose `DepField` targets resolve to done records. Unresolved dependency → blocks + warning. Output sorted by `Path`.

- [ ] **Step 1: Write the failing test**

```go
// internal/graph/next_test.go
package graph

import "testing"

func task(path, status string, deps ...string) recArg {
	fm := map[string]any{"id": path, "status": status}
	if len(deps) > 0 {
		d := make([]any, len(deps))
		for i, x := range deps {
			d[i] = x
		}
		fm["dependencies"] = d
	}
	return recArg{path: path, fm: fm}
}

type recArg struct {
	path string
	fm   map[string]any
}

func buildTasks(args ...recArg) *Graph {
	recs := make([]recArg, 0, len(args))
	recs = append(recs, args...)
	var fvs []recArg = recs
	_ = fvs
	var list []recArg = recs
	_ = list
	var rs []recArg = args
	_ = rs
	var out []recArg = args
	_ = out
	var records []recArg = args
	_ = records
	var built []recArg = args
	_ = built
	// build model records
	var fv []recArgConverted
	_ = fv
	return Build(convert(args), DefaultOptions())
}
```

> NOTE TO IMPLEMENTER: the helper soup above is intentionally a placeholder to be replaced — write the clean version below instead. Delete the messy `buildTasks` draft and use this:

```go
// internal/graph/next_test.go  (clean version — use this)
package graph

import (
	"testing"

	"tfq/internal/model"
)

func taskRec(id, status string, deps ...string) model.FileVitals {
	fm := map[string]any{"id": id, "status": status}
	if len(deps) > 0 {
		d := make([]any, len(deps))
		for i, x := range deps {
			d[i] = x
		}
		fm["dependencies"] = d
	}
	return model.FileVitals{
		Path: id + ".md", Format: "markdown", Frontmatter: fm,
		Headings: []model.Heading{}, Links: []model.Link{},
		Markers: []model.Marker{}, Warnings: []model.Warning{},
	}
}

func TestNextRespectsBlocking(t *testing.T) {
	// 001 done; 002 depends on 001 (ready); 003 depends on 002 (blocked); 004 plain note (no status)
	recs := []model.FileVitals{
		taskRec("001", "completed"),
		taskRec("002", "pending", "001"),
		taskRec("003", "pending", "002"),
		{Path: "note.md", Format: "markdown", Frontmatter: map[string]any{"slug": "note"},
			Headings: []model.Heading{}, Links: []model.Link{}, Markers: []model.Marker{}, Warnings: []model.Warning{}},
	}
	g := Build(recs, DefaultOptions())
	ready, _ := g.Next(DefaultNextOptions())

	if len(ready) != 1 {
		t.Fatalf("expected 1 ready task, got %d: %#v", len(ready), ready)
	}
	if ready[0].Path != "002.md" {
		t.Errorf("ready task = %q, want 002.md", ready[0].Path)
	}
}

func TestNextUnresolvedDepBlocksWithWarning(t *testing.T) {
	recs := []model.FileVitals{taskRec("005", "pending", "ghost")}
	g := Build(recs, DefaultOptions())
	ready, warns := g.Next(DefaultNextOptions())
	if len(ready) != 0 {
		t.Errorf("task with unresolved dep should be blocked, got %#v", ready)
	}
	if len(warns) == 0 {
		t.Errorf("expected a warning for unresolved dependency")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/graph/... -run TestNext`
Expected: FAIL (`Next`/`DefaultNextOptions` undefined).

- [ ] **Step 3: Write the implementation**

```go
// internal/graph/next.go
package graph

import (
	"sort"

	"tfq/internal/model"
)

// NextOptions configures the dependency-aware ready-set computation.
type NextOptions struct {
	DepField     string
	StatusField  string
	DoneStatuses []string
}

// DefaultNextOptions returns the conventional taskmd-compatible settings.
func DefaultNextOptions() NextOptions {
	return NextOptions{
		DepField:     "dependencies",
		StatusField:  "status",
		DoneStatuses: []string{"completed", "done", "cancelled"},
	}
}

// Next returns the records that are ready to work on: tasks (records with the
// status field) that are not done and whose dependencies are all satisfied.
func (g *Graph) Next(o NextOptions) ([]model.FileVitals, []model.Warning) {
	done := map[string]bool{}
	for _, s := range o.DoneStatuses {
		done[s] = true
	}
	status := func(r model.FileVitals) (string, bool) {
		return fmString(r.Frontmatter, o.StatusField)
	}

	ready := []model.FileVitals{}
	var warns []model.Warning
	for _, r := range g.records {
		st, isTask := status(r)
		if !isTask || done[st] {
			continue
		}
		blocked := false
		for _, raw := range edgeValues(r.Frontmatter[o.DepField]) {
			to, ok := g.Resolve(raw)
			if !ok {
				warns = append(warns, model.Warning{Module: "next", Message: r.Path + ": unresolved dependency " + raw})
				blocked = true
				continue
			}
			depStatus := ""
			for _, dr := range g.records {
				if dr.Path == to {
					depStatus, _ = fmString(dr.Frontmatter, o.StatusField)
					break
				}
			}
			if !done[depStatus] {
				blocked = true
			}
		}
		if !blocked {
			ready = append(ready, r)
		}
	}
	sort.Slice(ready, func(i, j int) bool { return ready[i].Path < ready[j].Path })
	return ready, warns
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/graph/... -run TestNext`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/graph/next.go internal/graph/next_test.go
git commit -m "feat(graph): dependency-aware next (blocking ready set)"
```

---

### Task 5: ripgrep-backed search with frontmatter filters

**Files:**
- Create: `internal/search/search.go`
- Test: `internal/search/search_test.go`

**Interfaces:**
- Consumes: `os/exec` (`rg`), `engine.InspectContent`, `model`.
- Produces:
  - `type Hit struct { Path string; Line int; Text string }` (JSON `path,line,text`)
  - `type Filters struct { Type string; Tag string }`
  - `func Search(root, query string, f Filters) ([]Hit, []model.Warning, error)`.
    - Runs `rg --json --line-number -- <query> <root>` and parses match events. `Path` is relative to `root` (slash-separated), `Line` is 1-based, `Text` is the matched line (trailing newline trimmed).
    - If `f.Type` set: keep a hit only if its file's frontmatter `type` equals `f.Type`.
    - If `f.Tag` set: keep a hit only if its file has a hashtag marker equal to `f.Tag` **or** frontmatter `tags` containing `f.Tag`.
    - `rg` exit code 1 (no matches) is **not** an error — return empty hits. Exit code ≥2 is an error.
    - Hits sorted by `Path` then `Line`.

- [ ] **Step 1: Write the failing test**

```go
// internal/search/search_test.go
package search

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSearchPlain(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.md", "---\ntype: note\n---\nhello world\nfoo bar\n")
	writeFile(t, dir, "b.md", "---\ntype: log\n---\nhello again\n")

	hits, _, err := Search(dir, "hello", Filters{})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d: %#v", len(hits), hits)
	}
	if hits[0].Path != "a.md" || hits[0].Line != 4 {
		t.Errorf("hit0 = %#v", hits[0])
	}
}

func TestSearchTypeFilter(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.md", "---\ntype: note\n---\nhello world\n")
	writeFile(t, dir, "b.md", "---\ntype: log\n---\nhello again\n")

	hits, _, err := Search(dir, "hello", Filters{Type: "log"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Path != "b.md" {
		t.Errorf("type filter wrong: %#v", hits)
	}
}

func TestSearchNoMatches(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.md", "nothing here\n")
	hits, _, err := Search(dir, "zzzznomatch", Filters{})
	if err != nil {
		t.Fatalf("no-match must not error: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("expected 0 hits, got %#v", hits)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/search/...`
Expected: FAIL (`Search` undefined).

- [ ] **Step 3: Write the implementation**

```go
// internal/search/search.go
package search

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"tfq/internal/engine"
	"tfq/internal/model"
)

// Hit is a single ripgrep match.
type Hit struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

// Filters narrows hits by frontmatter.
type Filters struct {
	Type string
	Tag  string
}

type rgEvent struct {
	Type string `json:"type"`
	Data struct {
		Path struct {
			Text string `json:"text"`
		} `json:"path"`
		LineNumber int `json:"line_number"`
		Lines      struct {
			Text string `json:"text"`
		} `json:"lines"`
	} `json:"data"`
}

// Search runs ripgrep over root and post-filters by frontmatter.
func Search(root, query string, f Filters) ([]Hit, []model.Warning, error) {
	cmd := exec.Command("rg", "--json", "--line-number", "--", query, root)
	out, err := cmd.Output()
	if err != nil {
		// rg exits 1 when there are no matches; that is not an error.
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return []Hit{}, nil, nil
		}
		return nil, nil, err
	}

	var warns []model.Warning
	cache := map[string]model.FileVitals{}
	inspect := func(abs string) (model.FileVitals, bool) {
		if v, ok := cache[abs]; ok {
			return v, true
		}
		b, rerr := os.ReadFile(abs)
		if rerr != nil {
			warns = append(warns, model.Warning{Module: "search", Message: "cannot read " + abs})
			return model.FileVitals{}, false
		}
		v := engine.InspectContent(abs, string(b))
		cache[abs] = v
		return v, true
	}

	hits := []Hit{}
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		var ev rgEvent
		if json.Unmarshal([]byte(line), &ev) != nil || ev.Type != "match" {
			continue
		}
		abs := ev.Data.Path.Text
		if f.Type != "" || f.Tag != "" {
			v, ok := inspect(abs)
			if !ok || !passesFilters(v, f) {
				continue
			}
		}
		rel, rerr := filepath.Rel(root, abs)
		if rerr != nil {
			rel = abs
		}
		hits = append(hits, Hit{
			Path: filepath.ToSlash(rel),
			Line: ev.Data.LineNumber,
			Text: strings.TrimRight(ev.Data.Lines.Text, "\n"),
		})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Path != hits[j].Path {
			return hits[i].Path < hits[j].Path
		}
		return hits[i].Line < hits[j].Line
	})
	return hits, warns, nil
}

func passesFilters(v model.FileVitals, f Filters) bool {
	if f.Type != "" {
		t, ok := v.Frontmatter["type"].(string)
		if !ok || t != f.Type {
			return false
		}
	}
	if f.Tag != "" {
		if !hasTag(v, f.Tag) {
			return false
		}
	}
	return true
}

func hasTag(v model.FileVitals, tag string) bool {
	for _, m := range v.Markers {
		if m.Kind == model.MarkerHashtag && m.Value == tag {
			return true
		}
	}
	switch t := v.Frontmatter["tags"].(type) {
	case []any:
		for _, e := range t {
			if s, ok := e.(string); ok && s == tag {
				return true
			}
		}
	case []string:
		for _, s := range t {
			if s == tag {
				return true
			}
		}
	}
	return false
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/search/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/search
git commit -m "feat(search): ripgrep-backed search with frontmatter filters"
```

---

### Task 6: Output schemas + gate tests for the new modes

**Files:**
- Create: `internal/schema/edges.schema.json` (graph edges output)
- Create: `internal/schema/hits.schema.json` (search output)
- Modify: `internal/schema/schema.go` (embed + validators)
- Test: `internal/schema/corpus_schema_test.go`

**Interfaces:**
- Produces:
  - `var EdgesSchema []byte`, `var HitsSchema []byte` (embedded)
  - `func ValidateEdges(edges []graph.Edge) error`
  - `func ValidateHits(hits []search.Hit) error`
  - (`next` output is `[]model.FileVitals`; validate each element with the existing `ValidateFileVitals`.)

- [ ] **Step 1: Write the schemas**

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://tfq/schemas/edges.json",
  "title": "Edges",
  "type": "array",
  "items": {
    "type": "object",
    "additionalProperties": false,
    "required": ["from", "kind", "raw", "to"],
    "properties": {
      "from": { "type": "string" },
      "kind": { "type": "string" },
      "raw": { "type": "string" },
      "to": { "type": "string" }
    }
  }
}
```

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://tfq/schemas/hits.json",
  "title": "Hits",
  "type": "array",
  "items": {
    "type": "object",
    "additionalProperties": false,
    "required": ["path", "line", "text"],
    "properties": {
      "path": { "type": "string" },
      "line": { "type": "integer", "minimum": 1 },
      "text": { "type": "string" }
    }
  }
}
```

- [ ] **Step 2: Write the failing test**

```go
// internal/schema/corpus_schema_test.go
package schema

import (
	"os"
	"path/filepath"
	"testing"

	"tfq/internal/graph"
	"tfq/internal/scan"
	"tfq/internal/search"
)

func TestEdgesOutputMatchesSchema(t *testing.T) {
	recs, _, err := scan.Collect(filepath.Join("..", "scan", "testdata", "vault"))
	if err != nil {
		t.Fatal(err)
	}
	g := graph.Build(recs, graph.DefaultOptions())
	if err := ValidateEdges(g.Edges()); err != nil {
		t.Errorf("edges schema violation: %v", err)
	}
	// next output is []FileVitals; each must pass the FileVitals schema
	ready, _ := g.Next(graph.DefaultNextOptions())
	for _, r := range ready {
		if err := ValidateFileVitals(r); err != nil {
			t.Errorf("next item schema violation: %v", err)
		}
	}
}

func TestHitsOutputMatchesSchema(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.md"), []byte("hello there\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hits, _, err := search.Search(dir, "hello", search.Filters{})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateHits(hits); err != nil {
		t.Errorf("hits schema violation: %v", err)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/schema/... -run 'TestEdges|TestHits'`
Expected: FAIL (`ValidateEdges`/`ValidateHits` undefined).

- [ ] **Step 4: Add the implementation to `internal/schema/schema.go`**

Append the new embeds, compiled schemas, and validators (keep the existing `FileVitalsSchema` block unchanged):

```go
//go:embed edges.schema.json
var EdgesSchema []byte

//go:embed hits.schema.json
var HitsSchema []byte

var compiledEdges = mustCompileNamed("edges.schema.json", EdgesSchema)
var compiledHits = mustCompileNamed("hits.schema.json", HitsSchema)

func mustCompileNamed(name string, src []byte) *jsonschema.Schema {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(src))
	if err != nil {
		panic(fmt.Sprintf("%s not valid json: %v", name, err))
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(name, doc); err != nil {
		panic(err)
	}
	s, err := c.Compile(name)
	if err != nil {
		panic(err)
	}
	return s
}

func validateAgainst(s *jsonschema.Schema, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
	if err != nil {
		return err
	}
	return s.Validate(inst)
}

// ValidateEdges validates graph edge output against the embedded schema.
func ValidateEdges(edges any) error { return validateAgainst(compiledEdges, edges) }

// ValidateHits validates search hit output against the embedded schema.
func ValidateHits(hits any) error { return validateAgainst(compiledHits, hits) }
```

> Implementer note: `ValidateEdges`/`ValidateHits` take `any` to avoid importing `graph`/`search` into `schema` (prevents an import cycle, since neither of those imports `schema`). The test passes concrete slices; JSON marshalling handles the shape.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/schema/...`
Expected: PASS (existing FileVitals gate + new edges/hits gates).

- [ ] **Step 6: Commit**

```bash
git add internal/schema
git commit -m "feat(schema): output gates for graph edges and search hits"
```

---

### Task 7: CLI verbs (backlinks, next, search, graph)

**Files:**
- Modify: `cmd/tfq/main.go`
- Test: `cmd/tfq/main_test.go` (add cases)

**Interfaces:**
- Consumes: `scan.Collect`, `graph.Build/DefaultOptions/DefaultNextOptions`, `search.Search`, `engine.Inspect`.
- Produces CLI verbs (all emit indented JSON on stdout, exit 0; usage/exit 2 on arg errors; runtime error/exit 1):
  - `tfq backlinks <ref> <dir>` → JSON array of source paths
  - `tfq next <dir>` → JSON array of `FileVitals`
  - `tfq search <query> <dir>` → JSON array of `Hit`
  - `tfq graph <dir>` → JSON array of `Edge`
  - existing `tfq inspect <file>` unchanged

- [ ] **Step 1: Write the failing test (add to `cmd/tfq/main_test.go`)**

```go
func TestRunBacklinksAndNext(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "001.md", "---\nid: \"001\"\nstatus: completed\n---\n# done\n")
	mustWrite(t, dir, "002.md", "---\nid: \"002\"\nstatus: pending\ndependencies: [\"001\"]\n---\n# go\nsee [[001]]\n")

	out, code := run([]string{"next", dir})
	if code != 0 {
		t.Fatalf("next exit %d: %s", code, out)
	}
	if !contains(out, "002.md") || contains(out, "001.md") {
		t.Errorf("next output wrong: %s", out)
	}

	out, code = run([]string{"backlinks", "001", dir})
	if code != 0 {
		t.Fatalf("backlinks exit %d: %s", code, out)
	}
	if !contains(out, "002.md") {
		t.Errorf("backlinks output wrong: %s", out)
	}
}

func TestRunSearch(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "a.md", "needle in here\n")
	out, code := run([]string{"search", "needle", dir})
	if code != 0 {
		t.Fatalf("search exit %d: %s", code, out)
	}
	if !contains(out, "a.md") {
		t.Errorf("search output wrong: %s", out)
	}
}

// helpers (add once to the test file)
func mustWrite(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
func contains(s, sub string) bool { return strings.Contains(s, sub) }
```

(Add imports `"strings"` to the test file if not present.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/tfq/...`
Expected: FAIL (unknown subcommands return usage/exit 2, so assertions fail).

- [ ] **Step 3: Extend `run` in `cmd/tfq/main.go`**

Replace the `switch` in `run` with the expanded version (keep `inspect` as-is):

```go
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
		return mustJSON(fv), 0
	case "graph":
		if len(args) != 2 {
			return usage(), 2
		}
		g, err := buildGraph(args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, "tfq: "+err.Error())
			return "", 1
		}
		return mustJSON(g.Edges()), 0
	case "backlinks":
		if len(args) != 3 {
			return usage(), 2
		}
		g, err := buildGraph(args[2])
		if err != nil {
			fmt.Fprintln(os.Stderr, "tfq: "+err.Error())
			return "", 1
		}
		return mustJSON(g.Backlinks(args[1])), 0
	case "next":
		if len(args) != 2 {
			return usage(), 2
		}
		g, err := buildGraph(args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, "tfq: "+err.Error())
			return "", 1
		}
		ready, _ := g.Next(graph.DefaultNextOptions())
		return mustJSON(ready), 0
	case "search":
		if len(args) != 3 {
			return usage(), 2
		}
		hits, _, err := search.Search(args[2], args[1], search.Filters{})
		if err != nil {
			fmt.Fprintln(os.Stderr, "tfq: "+err.Error())
			return "", 1
		}
		return mustJSON(hits), 0
	default:
		return usage(), 2
	}
```

Add these helpers and imports to `main.go`:

```go
// (imports) add: "tfq/internal/graph", "tfq/internal/scan", "tfq/internal/search"

func buildGraph(dir string) (*graph.Graph, error) {
	recs, _, err := scan.Collect(dir)
	if err != nil {
		return nil, err
	}
	return graph.Build(recs, graph.DefaultOptions()), nil
}

func mustJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(b)
}
```

Update `usage()`:

```go
func usage() string {
	return "usage: tfq <inspect <file> | graph <dir> | backlinks <ref> <dir> | next <dir> | search <query> <dir>>"
}
```

- [ ] **Step 4: Run tests + build + smoke**

Run: `go test ./cmd/tfq/...`
Expected: PASS.

Run: `go build -o tfq ./cmd/tfq && ./tfq next internal/scan/testdata/vault; ./tfq graph internal/scan/testdata/vault | head -12`
Expected: `next` prints `[]` or tasks; `graph` prints an edges array (note-a → note-b wiki edge resolved).

- [ ] **Step 5: Final full-suite run and commit**

Run: `go vet ./... && go test ./...`
Expected: all packages PASS.

```bash
git add cmd/tfq
git commit -m "feat(cmd): graph, backlinks, next, search subcommands"
```

---

## Self-Review

- **Spec coverage (Phase 2):** rg-backed search (Task 5) ✓; graph from extracted edges (Task 2) ✓; backlinks (Task 3) ✓; dependency-aware `next` (Task 4) ✓; per-mode output schemas validated in tests (Task 6) ✓; thin CLI exposure consistent with deferred vocabulary (Task 7) ✓; liberal/never-fail + dangling-as-warning (Tasks 2/4) ✓; no index, no semantic search (search is pure rg) ✓.
- **Placeholders:** Task 4 Step 1 contains an intentional "helper soup" draft immediately followed by the clean version to use — the implementer deletes the draft. All other steps have runnable code + expected output.
- **Type consistency:** `model.FileVitals`, `scan.Collect`, `graph.Build/Edge/Options/DefaultOptions/Resolve/Edges/Warnings/Backlinks/Forward/Next/DefaultNextOptions`, `search.Search/Hit/Filters`, and `schema.ValidateEdges/ValidateHits` are used identically across producing and consuming tasks. `ValidateEdges/ValidateHits` take `any` to avoid a `schema → graph/search` import cycle.
