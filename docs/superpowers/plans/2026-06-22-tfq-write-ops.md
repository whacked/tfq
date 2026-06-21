# tfq Write Operations Implementation Plan (Phase 4b — write ops)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Add the operations that let `tfq` replace `ov`/`taskmd`: `read` and `list` (read-only), plus `new` and `set` (writes). All path/sharding/ID logic lives in a single config-shaped `layout` package so future user-supplied rules plug in without touching call sites.

**Architecture:** A `layout` package owns the path policy: token-pattern rules per template (`note`/`task`) with daily vs global sequencing — defaults replicate the agent-resources conventions, but it is one configurable seam. A `query` package provides read-only `List` (filtered projection) and `Read` (resolve a ref → record + body). A `store` package provides `New` (create via layout + frontmatter template) and `Set` (mutate frontmatter in place via a yaml.Node round-trip that preserves body + key order). Each new output mode gets a JSON Schema gated in tests. CLI verbs stay flat: `read`, `list`, `new`, `set`.

**Tech Stack:** Go 1.25, `gopkg.in/yaml.v3` (Node round-trip). Builds on `model`, `scan`, `graph`, `engine`, `schema`.

## Global Constraints

- **Path policy is centralized in `layout`** — no sharding/ID/filename logic anywhere else. `layout.Config` is a plain struct (future-loadable from config); `DefaultConfig()` encodes today's conventions.
- **Default conventions (agent-resources-compatible):**
  - note → dir `{yyyy}/{mm}`, file `{yyyy}-{mm}-{dd}.{nnn}-{slug}.md`, **daily** sequence, padding 3.
  - task → dir `{yyyy}/{mm}`, file `{nnn}-{slug}.md`, **global** sequence, padding 3.
- **Determinism:** all path/sequence/create functions take an explicit `date time.Time`; only the CLI calls `time.Now()`. Tests pass fixed dates and use `t.TempDir()`.
- **`set` preserves the body and existing frontmatter key order** (yaml.Node round-trip), never rewrites the whole file from a map.
- Slug rule: `[a-z0-9-]+` (reject otherwise). `<ref>` resolves by any key (path/basename/id/slug/title) via the existing graph resolver.
- All output is JSON and schema-gated in tests. Exit codes: 0 success, 1 runtime error, 2 usage error.
- No changes outside this repo (agent-resources fold-in is still deferred).

---

### Task 1: layout — the path-policy config seam

**Files:**
- Create: `internal/layout/layout.go`
- Test: `internal/layout/layout_test.go`

**Interfaces:**
- Produces:
  - `type Template string` with consts `TemplateNote = "note"`, `TemplateTask = "task"`
  - `type Rule struct { Dir, File, Sequence string; Padding int }`
  - `type Config struct { Rules map[Template]Rule }`
  - `func DefaultConfig() Config`
  - `func (c Config) RelPath(tmpl Template, slug string, date time.Time, seq int) (string, error)` — token substitution `{yyyy}{mm}{dd}{nnn}{slug}`, slash-joined `Dir/File`. Unknown template → error.
  - `func (c Config) NextSequence(root string, tmpl Template, date time.Time) (int, error)` — `daily`: 1 + max NNN among files in the shard dir whose name starts with `{yyyy}-{mm}-{dd}.`; `global`: 1 + max leading-integer among `*.md` basenames anywhere under root. Empty/missing → 1.

- [ ] **Step 1: Write the failing test**

```go
// internal/layout/layout_test.go
package layout

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func date(t *testing.T) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", "2026-06-22")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestRelPath(t *testing.T) {
	c := DefaultConfig()
	d := date(t)
	note, err := c.RelPath(TemplateNote, "my-slug", d, 1)
	if err != nil {
		t.Fatal(err)
	}
	if note != "2026/06/2026-06-22.001-my-slug.md" {
		t.Errorf("note path = %q", note)
	}
	task, err := c.RelPath(TemplateTask, "do-thing", d, 4)
	if err != nil {
		t.Fatal(err)
	}
	if task != "2026/06/004-do-thing.md" {
		t.Errorf("task path = %q", task)
	}
	if _, err := c.RelPath("bogus", "x", d, 1); err == nil {
		t.Error("unknown template should error")
	}
}

func TestNextSequenceDaily(t *testing.T) {
	root := t.TempDir()
	c := DefaultConfig()
	d := date(t)
	// no files yet -> 1
	n, err := c.NextSequence(root, TemplateNote, d)
	if err != nil || n != 1 {
		t.Fatalf("empty daily seq = %d (%v)", n, err)
	}
	// create today's 001 and 002 in the shard
	shard := filepath.Join(root, "2026", "06")
	if err := os.MkdirAll(shard, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"2026-06-22.001-a.md", "2026-06-22.002-b.md", "2026-06-21.005-old.md"} {
		if err := os.WriteFile(filepath.Join(shard, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	n, _ = c.NextSequence(root, TemplateNote, d)
	if n != 3 {
		t.Errorf("daily seq = %d, want 3 (yesterday's 005 must not count)", n)
	}
}

func TestNextSequenceGlobal(t *testing.T) {
	root := t.TempDir()
	c := DefaultConfig()
	d := date(t)
	shard := filepath.Join(root, "2026", "05")
	if err := os.MkdirAll(shard, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"003-x.md", "007-y.md"} {
		if err := os.WriteFile(filepath.Join(shard, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	n, _ := c.NextSequence(root, TemplateTask, d)
	if n != 8 {
		t.Errorf("global seq = %d, want 8", n)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/layout/...`
Expected: FAIL (undefined).

- [ ] **Step 3: Write the implementation**

```go
// internal/layout/layout.go
package layout

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Template selects a path rule.
type Template string

const (
	TemplateNote Template = "note"
	TemplateTask Template = "task"
)

// Rule is the path policy for one template.
type Rule struct {
	Dir      string // pattern, e.g. "{yyyy}/{mm}"
	File     string // pattern, e.g. "{yyyy}-{mm}-{dd}.{nnn}-{slug}.md"
	Sequence string // "daily" | "global"
	Padding  int
}

// Config is the full path policy. Plain struct so it can be loaded from
// user config in the future; DefaultConfig encodes today's conventions.
type Config struct {
	Rules map[Template]Rule
}

// DefaultConfig replicates the agent-resources note/task conventions.
func DefaultConfig() Config {
	return Config{Rules: map[Template]Rule{
		TemplateNote: {Dir: "{yyyy}/{mm}", File: "{yyyy}-{mm}-{dd}.{nnn}-{slug}.md", Sequence: "daily", Padding: 3},
		TemplateTask: {Dir: "{yyyy}/{mm}", File: "{nnn}-{slug}.md", Sequence: "global", Padding: 3},
	}}
}

func (c Config) rule(tmpl Template) (Rule, error) {
	r, ok := c.Rules[tmpl]
	if !ok {
		return Rule{}, fmt.Errorf("unknown template %q", tmpl)
	}
	return r, nil
}

func subst(pattern string, date time.Time, seq, padding int, slug string) string {
	rep := strings.NewReplacer(
		"{yyyy}", date.Format("2006"),
		"{mm}", date.Format("01"),
		"{dd}", date.Format("02"),
		"{nnn}", fmt.Sprintf("%0*d", padding, seq),
		"{slug}", slug,
	)
	return rep.Replace(pattern)
}

// RelPath computes the collection-relative path for a new record.
func (c Config) RelPath(tmpl Template, slug string, date time.Time, seq int) (string, error) {
	r, err := c.rule(tmpl)
	if err != nil {
		return "", err
	}
	dir := subst(r.Dir, date, seq, r.Padding, slug)
	file := subst(r.File, date, seq, r.Padding, slug)
	return filepath.ToSlash(filepath.Join(dir, file)), nil
}

var leadingInt = regexp.MustCompile(`^(\d+)-`)

// NextSequence computes the next sequence number under root for the template.
func (c Config) NextSequence(root string, tmpl Template, date time.Time) (int, error) {
	r, err := c.rule(tmpl)
	if err != nil {
		return 0, err
	}
	max := 0
	if r.Sequence == "daily" {
		shard := filepath.Join(root, subst(r.Dir, date, 0, r.Padding, ""))
		prefix := date.Format("2006-01-02") + "."
		entries, derr := filepath.Glob(filepath.Join(shard, prefix+"*"))
		if derr != nil {
			return 0, derr
		}
		re := regexp.MustCompile(`\.(\d+)-`)
		for _, p := range entries {
			if m := re.FindStringSubmatch(filepath.Base(p)); m != nil {
				if n, _ := strconv.Atoi(m[1]); n > max {
					max = n
				}
			}
		}
		return max + 1, nil
	}
	// global: max leading integer among *.md basenames under root
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		if m := leadingInt.FindStringSubmatch(d.Name()); m != nil {
			if n, _ := strconv.Atoi(m[1]); n > max {
				max = n
			}
		}
		return nil
	})
	return max + 1, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/layout/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/layout
git commit -m "feat(layout): centralized path/sharding/sequence config seam"
```

---

### Task 2: query — read-only List and Read

**Files:**
- Create: `internal/query/query.go`
- Test: `internal/query/query_test.go`

**Interfaces:**
- Consumes: `scan.Collect`, `graph.Build/DefaultOptions`, `model`.
- Produces:
  - `type ListItem struct { Path, Title, Status, Type string; Tags []string }` (JSON `path,title,status,type,tags`; Tags non-nil)
  - `type ListFilters struct { Status, Tag, Type string }`
  - `func List(root string, f ListFilters) ([]ListItem, error)` — projects each record; `Title` = frontmatter `title`, else `slug`, else first heading text, else `""`. Filters are AND; empty filter matches all. Sorted by Path.
  - `type Record struct { Path, Format string; Frontmatter map[string]any; Body string }` (JSON `path,format,frontmatter,body`)
  - `func Read(root, ref string) (Record, error)` — resolve ref via graph; read file; `Body` is the content after the frontmatter block (or whole content if none). Unknown ref → error.

- [ ] **Step 1: Write fixtures + failing test**

```go
// internal/query/query_test.go
package query

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

func TestListFilters(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "001.md", "---\nid: \"001\"\nstatus: pending\ntype: task\ntags: [a]\n---\n# One\n")
	writeFile(t, dir, "002.md", "---\nid: \"002\"\nstatus: completed\ntype: task\n---\n# Two\n")

	all, err := List(dir, ListFilters{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("want 2, got %d", len(all))
	}

	pend, _ := List(dir, ListFilters{Status: "pending"})
	if len(pend) != 1 || pend[0].Path != "001.md" || pend[0].Title != "One" {
		t.Errorf("status filter wrong: %#v", pend)
	}

	tagged, _ := List(dir, ListFilters{Tag: "a"})
	if len(tagged) != 1 || tagged[0].Path != "001.md" {
		t.Errorf("tag filter wrong: %#v", tagged)
	}
}

func TestRead(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "note.md", "---\nslug: hi\n---\n# Heading\nbody line\n")
	r, err := Read(dir, "hi")
	if err != nil {
		t.Fatal(err)
	}
	if r.Path != "note.md" {
		t.Errorf("path = %q", r.Path)
	}
	if r.Frontmatter["slug"] != "hi" {
		t.Errorf("frontmatter = %#v", r.Frontmatter)
	}
	if want := "# Heading\nbody line\n"; r.Body != want {
		t.Errorf("body = %q, want %q", r.Body, want)
	}
	if _, err := Read(dir, "nonexistent"); err == nil {
		t.Error("unknown ref should error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/query/...`
Expected: FAIL (undefined).

- [ ] **Step 3: Write the implementation**

```go
// internal/query/query.go
package query

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"tfq/internal/graph"
	"tfq/internal/model"
	"tfq/internal/scan"
)

// ListItem is a compact record summary.
type ListItem struct {
	Path   string   `json:"path"`
	Title  string   `json:"title"`
	Status string   `json:"status"`
	Type   string   `json:"type"`
	Tags   []string `json:"tags"`
}

// ListFilters narrows a List (AND semantics; empty matches all).
type ListFilters struct {
	Status string
	Tag    string
	Type   string
}

func fmStr(fm map[string]any, key string) string {
	if s, ok := fm[key].(string); ok {
		return s
	}
	return ""
}

func fmTags(fm map[string]any) []string {
	out := []string{}
	switch t := fm["tags"].(type) {
	case []any:
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
	case []string:
		out = append(out, t...)
	}
	return out
}

func titleOf(r model.FileVitals) string {
	if t := fmStr(r.Frontmatter, "title"); t != "" {
		return t
	}
	if s := fmStr(r.Frontmatter, "slug"); s != "" {
		return s
	}
	if len(r.Headings) > 0 {
		return r.Headings[0].Text
	}
	return ""
}

// List returns filtered record summaries under root.
func List(root string, f ListFilters) ([]ListItem, error) {
	recs, _, err := scan.Collect(root)
	if err != nil {
		return nil, err
	}
	out := []ListItem{}
	for _, r := range recs {
		tags := fmTags(r.Frontmatter)
		if f.Status != "" && fmStr(r.Frontmatter, "status") != f.Status {
			continue
		}
		if f.Type != "" && fmStr(r.Frontmatter, "type") != f.Type {
			continue
		}
		if f.Tag != "" && !containsStr(tags, f.Tag) {
			continue
		}
		out = append(out, ListItem{
			Path:   r.Path,
			Title:  titleOf(r),
			Status: fmStr(r.Frontmatter, "status"),
			Type:   fmStr(r.Frontmatter, "type"),
			Tags:   tags,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func containsStr(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// Record is a full record including body.
type Record struct {
	Path        string         `json:"path"`
	Format      string         `json:"format"`
	Frontmatter map[string]any `json:"frontmatter"`
	Body        string         `json:"body"`
}

// Read resolves ref under root and returns the record with its body.
func Read(root, ref string) (Record, error) {
	recs, _, err := scan.Collect(root)
	if err != nil {
		return Record{}, err
	}
	g := graph.Build(recs, graph.DefaultOptions())
	rel, ok := g.Resolve(ref)
	if !ok {
		return Record{}, fmt.Errorf("no record matches %q", ref)
	}
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return Record{}, err
	}
	content := string(b)
	var target model.FileVitals
	for _, r := range recs {
		if r.Path == rel {
			target = r
			break
		}
	}
	return Record{Path: rel, Format: target.Format, Frontmatter: target.Frontmatter, Body: bodyAfterFrontmatter(content)}, nil
}

// bodyAfterFrontmatter returns content after a leading --- ... --- block.
func bodyAfterFrontmatter(content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r") != "---" {
		return content
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r") == "---" {
			return strings.Join(lines[i+1:], "\n")
		}
	}
	return content
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/query/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/query
git commit -m "feat(query): read-only List (filtered) and Read (ref -> record+body)"
```

---

### Task 3: store.New — create a record via layout + template

**Files:**
- Create: `internal/store/store.go`
- Test: `internal/store/new_test.go`

**Interfaces:**
- Consumes: `layout`, `model`.
- Produces:
  - `type WriteResult struct { Path, Action string }` (JSON `path,action`)
  - `func New(root string, tmpl layout.Template, slug string, fields map[string]string, date time.Time, cfg layout.Config) (WriteResult, error)` — validates slug `[a-z0-9-]+`; computes seq + relpath via cfg; builds frontmatter + body from a per-template scaffold; merges `fields` into frontmatter (string values); refuses to overwrite an existing file; returns `{relpath, "created"}`.

  Scaffolds:
  - note: frontmatter `date`(YYYY-MM-DD), `author: agent`, `slug`, `source_notes: []`, `tags: []`; body `# <slug words>\n\n<summary>\n`.
  - task: frontmatter `id`(the padded seq), `title: <slug words>`, `status: pending`, `priority: medium`; body `# <slug words>\n`.

- [ ] **Step 1: Write the failing test**

```go
// internal/store/new_test.go
package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tfq/internal/layout"
)

func fixedDate(t *testing.T) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", "2026-06-22")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestNewNote(t *testing.T) {
	root := t.TempDir()
	res, err := New(root, layout.TemplateNote, "my-idea", nil, fixedDate(t), layout.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != "2026/06/2026-06-22.001-my-idea.md" || res.Action != "created" {
		t.Fatalf("result = %#v", res)
	}
	b, err := os.ReadFile(filepath.Join(root, res.Path))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "slug: my-idea") || !strings.Contains(s, "author: agent") {
		t.Errorf("note frontmatter wrong:\n%s", s)
	}
}

func TestNewTaskWithFields(t *testing.T) {
	root := t.TempDir()
	res, err := New(root, layout.TemplateTask, "do-thing", map[string]string{"priority": "high"}, fixedDate(t), layout.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != "2026/06/001-do-thing.md" {
		t.Fatalf("path = %q", res.Path)
	}
	b, _ := os.ReadFile(filepath.Join(root, res.Path))
	s := string(b)
	if !strings.Contains(s, "status: pending") || !strings.Contains(s, "priority: high") {
		t.Errorf("task frontmatter wrong:\n%s", s)
	}
	if !strings.Contains(s, "id:") {
		t.Errorf("task missing id:\n%s", s)
	}
}

func TestNewRejectsBadSlug(t *testing.T) {
	root := t.TempDir()
	if _, err := New(root, layout.TemplateNote, "Bad Slug", nil, fixedDate(t), layout.DefaultConfig()); err == nil {
		t.Error("expected error for invalid slug")
	}
}

func TestNewNoOverwrite(t *testing.T) {
	root := t.TempDir()
	cfg := layout.DefaultConfig()
	if _, err := New(root, layout.TemplateTask, "x", nil, fixedDate(t), cfg); err != nil {
		t.Fatal(err)
	}
	// a second create gets the next sequence, not an overwrite
	res, err := New(root, layout.TemplateTask, "y", nil, fixedDate(t), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != "2026/06/002-y.md" {
		t.Errorf("second task path = %q, want 002-y.md", res.Path)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/... -run TestNew`
Expected: FAIL (undefined).

- [ ] **Step 3: Write the implementation**

```go
// internal/store/store.go
package store

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"tfq/internal/layout"
)

// WriteResult reports a create/update.
type WriteResult struct {
	Path   string `json:"path"`
	Action string `json:"action"`
}

var slugRe = regexp.MustCompile(`^[a-z0-9-]+$`)

// New creates a record file under root using the layout config and a template.
func New(root string, tmpl layout.Template, slug string, fields map[string]string, date time.Time, cfg layout.Config) (WriteResult, error) {
	if !slugRe.MatchString(slug) {
		return WriteResult{}, fmt.Errorf("slug %q must match [a-z0-9-]+", slug)
	}
	seq, err := cfg.NextSequence(root, tmpl, date)
	if err != nil {
		return WriteResult{}, err
	}
	rel, err := cfg.RelPath(tmpl, slug, date, seq)
	if err != nil {
		return WriteResult{}, err
	}
	full := filepath.Join(root, rel)
	if _, err := os.Stat(full); err == nil {
		return WriteResult{}, fmt.Errorf("file already exists: %s", rel)
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return WriteResult{}, err
	}
	content := scaffold(tmpl, slug, date, seq, cfg, fields)
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		return WriteResult{}, err
	}
	return WriteResult{Path: rel, Action: "created"}, nil
}

func titleWords(slug string) string {
	return strings.ReplaceAll(slug, "-", " ")
}

// scaffold builds frontmatter + body for a new record. fields override defaults.
func scaffold(tmpl layout.Template, slug string, date time.Time, seq int, cfg layout.Config, fields map[string]string) string {
	pad := cfg.Rules[tmpl].Padding
	var fm []string
	var body string
	switch tmpl {
	case layout.TemplateTask:
		base := map[string]string{
			"id":       fmt.Sprintf("%0*d", pad, seq),
			"title":    titleWords(slug),
			"status":   "pending",
			"priority": "medium",
		}
		order := []string{"id", "title", "status", "priority"}
		fm = renderFM(base, order, fields)
		body = "# " + titleWords(slug) + "\n"
	default: // note
		base := map[string]string{
			"date":   date.Format("2006-01-02"),
			"author": "agent",
			"slug":   slug,
		}
		order := []string{"date", "author", "slug"}
		fm = renderFM(base, order, fields)
		fm = append(fm, "source_notes: []", "tags: []")
		body = "# " + titleWords(slug) + "\n\n<summary>\n"
	}
	return "---\n" + strings.Join(fm, "\n") + "\n---\n" + body
}

// renderFM emits "key: value" lines for base keys in order, with fields
// overriding values and any extra fields appended in sorted order.
func renderFM(base map[string]string, order []string, fields map[string]string) []string {
	out := []string{}
	used := map[string]bool{}
	for _, k := range order {
		v := base[k]
		if fields != nil {
			if ov, ok := fields[k]; ok {
				v = ov
			}
		}
		used[k] = true
		out = append(out, k+": "+v)
	}
	extra := []string{}
	for k := range fields {
		if !used[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	for _, k := range extra {
		out = append(out, k+": "+fields[k])
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/... -run TestNew`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/store.go internal/store/new_test.go
git commit -m "feat(store): New creates records via layout + template scaffold"
```

---

### Task 4: store.Set — mutate frontmatter preserving body + order

**Files:**
- Create: `internal/store/set.go`
- Test: `internal/store/set_test.go`

**Interfaces:**
- Consumes: `scan.Collect`, `graph`, `gopkg.in/yaml.v3`.
- Produces:
  - `func Set(root, ref string, changes map[string]string, addTags []string) (WriteResult, error)` — resolves ref; rewrites the frontmatter block via yaml.Node (set/replace scalar keys from `changes`; append `addTags` to a `tags` sequence, creating it if absent); preserves the body verbatim and existing key order; returns `{relpath, "updated"}`.

- [ ] **Step 1: Write the failing test**

```go
// internal/store/set_test.go
package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetStatusAndTag(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "001.md")
	if err := os.WriteFile(p, []byte("---\nid: \"001\"\nstatus: pending\ntags: [a]\n---\n# body\nkeep me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Set(root, "001", map[string]string{"status": "completed"}, []string{"reviewed"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != "001.md" || res.Action != "updated" {
		t.Fatalf("result = %#v", res)
	}
	b, _ := os.ReadFile(p)
	s := string(b)
	if !strings.Contains(s, "status: completed") {
		t.Errorf("status not updated:\n%s", s)
	}
	if !strings.Contains(s, "reviewed") {
		t.Errorf("tag not appended:\n%s", s)
	}
	// body preserved
	if !strings.Contains(s, "# body\nkeep me") {
		t.Errorf("body not preserved:\n%s", s)
	}
	// existing id key preserved
	if !strings.Contains(s, "id:") {
		t.Errorf("id key lost:\n%s", s)
	}
}

func TestSetAddsMissingKey(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "n.md")
	if err := os.WriteFile(p, []byte("---\nslug: n\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Set(root, "n", map[string]string{"status": "active"}, nil); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "status: active") {
		t.Errorf("missing key not added:\n%s", string(b))
	}
}

func TestSetUnknownRef(t *testing.T) {
	root := t.TempDir()
	if _, err := Set(root, "ghost", map[string]string{"x": "y"}, nil); err == nil {
		t.Error("unknown ref should error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/... -run TestSet`
Expected: FAIL (undefined).

- [ ] **Step 3: Write the implementation**

```go
// internal/store/set.go
package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
	"tfq/internal/graph"
	"tfq/internal/scan"
)

// Set mutates the frontmatter of the record ref resolves to, preserving body
// and existing key order.
func Set(root, ref string, changes map[string]string, addTags []string) (WriteResult, error) {
	recs, _, err := scan.Collect(root)
	if err != nil {
		return WriteResult{}, err
	}
	g := graph.Build(recs, graph.DefaultOptions())
	rel, ok := g.Resolve(ref)
	if !ok {
		return WriteResult{}, fmt.Errorf("no record matches %q", ref)
	}
	full := filepath.Join(root, rel)
	b, err := os.ReadFile(full)
	if err != nil {
		return WriteResult{}, err
	}
	updated, err := rewriteFrontmatter(string(b), changes, addTags)
	if err != nil {
		return WriteResult{}, err
	}
	if err := os.WriteFile(full, []byte(updated), 0o644); err != nil {
		return WriteResult{}, err
	}
	return WriteResult{Path: rel, Action: "updated"}, nil
}

// rewriteFrontmatter applies changes/addTags to the leading --- block.
func rewriteFrontmatter(content string, changes map[string]string, addTags []string) (string, error) {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r") != "---" {
		return "", fmt.Errorf("no frontmatter block to modify")
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r") == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return "", fmt.Errorf("unterminated frontmatter block")
	}
	fmSrc := strings.Join(lines[1:end], "\n")
	body := strings.Join(lines[end+1:], "\n")

	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(fmSrc), &doc); err != nil {
		return "", err
	}
	var mapping *yaml.Node
	if len(doc.Content) == 1 && doc.Content[0].Kind == yaml.MappingNode {
		mapping = doc.Content[0]
	} else {
		mapping = &yaml.Node{Kind: yaml.MappingNode}
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{mapping}}
	}

	for k, v := range changes {
		setScalar(mapping, k, v)
	}
	for _, tag := range addTags {
		appendTag(mapping, tag)
	}

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return "", err
	}
	return "---\n" + string(out) + "---\n" + body, nil
}

func findValue(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

func setScalar(mapping *yaml.Node, key, value string) {
	if v := findValue(mapping, key); v != nil {
		v.Kind = yaml.ScalarNode
		v.Tag = ""
		v.Value = value
		v.Content = nil
		return
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Value: value},
	)
}

func appendTag(mapping *yaml.Node, tag string) {
	v := findValue(mapping, "tags")
	if v == nil {
		seq := &yaml.Node{Kind: yaml.SequenceNode}
		seq.Content = append(seq.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: tag})
		mapping.Content = append(mapping.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "tags"}, seq)
		return
	}
	if v.Kind != yaml.SequenceNode {
		return
	}
	for _, e := range v.Content {
		if e.Value == tag {
			return
		}
	}
	v.Content = append(v.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: tag})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/store/set.go internal/store/set_test.go
git commit -m "feat(store): Set mutates frontmatter via yaml.Node, preserving body"
```

---

### Task 5: Output schemas + gates for list, record, write-result

**Files:**
- Create: `internal/schema/list.schema.json`, `internal/schema/record.schema.json`, `internal/schema/write.schema.json`
- Modify: `internal/schema/schema.go`
- Test: `internal/schema/writeops_schema_test.go`

**Interfaces:**
- Produces: `var ListSchema, RecordSchema, WriteSchema []byte`; `func ValidateList(any) error`, `func ValidateRecord(any) error`, `func ValidateWrite(any) error`.

- [ ] **Step 1: Write the schemas**

`list.schema.json`:
```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://tfq/schemas/list.json",
  "title": "List",
  "type": "array",
  "items": {
    "type": "object",
    "additionalProperties": false,
    "required": ["path", "title", "status", "type", "tags"],
    "properties": {
      "path": { "type": "string" },
      "title": { "type": "string" },
      "status": { "type": "string" },
      "type": { "type": "string" },
      "tags": { "type": "array", "items": { "type": "string" } }
    }
  }
}
```

`record.schema.json`:
```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://tfq/schemas/record.json",
  "title": "Record",
  "type": "object",
  "additionalProperties": false,
  "required": ["path", "format", "frontmatter", "body"],
  "properties": {
    "path": { "type": "string" },
    "format": { "type": "string" },
    "frontmatter": { "type": "object" },
    "body": { "type": "string" }
  }
}
```

`write.schema.json`:
```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://tfq/schemas/write.json",
  "title": "WriteResult",
  "type": "object",
  "additionalProperties": false,
  "required": ["path", "action"],
  "properties": {
    "path": { "type": "string" },
    "action": { "type": "string", "enum": ["created", "updated"] }
  }
}
```

- [ ] **Step 2: Write the failing test**

```go
// internal/schema/writeops_schema_test.go
package schema

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"tfq/internal/layout"
	"tfq/internal/query"
	"tfq/internal/store"
)

func TestWriteOpsSchemas(t *testing.T) {
	root := t.TempDir()
	d, _ := time.Parse("2006-01-02", "2026-06-22")

	w, err := store.New(root, layout.TemplateTask, "do-it", nil, d, layout.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateWrite(w); err != nil {
		t.Errorf("write schema violation: %v", err)
	}

	items, err := query.List(root, query.ListFilters{})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateList(items); err != nil {
		t.Errorf("list schema violation: %v", err)
	}

	// read the file we just created
	_ = os.WriteFile(filepath.Join(root, "extra.md"), []byte("---\nslug: e\n---\nbody\n"), 0o644)
	rec, err := query.Read(root, "e")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateRecord(rec); err != nil {
		t.Errorf("record schema violation: %v", err)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/schema/... -run TestWriteOps`
Expected: FAIL (validators undefined).

- [ ] **Step 4: Add to `internal/schema/schema.go`**

```go
//go:embed list.schema.json
var ListSchema []byte

//go:embed record.schema.json
var RecordSchema []byte

//go:embed write.schema.json
var WriteSchema []byte

var compiledList = mustCompileNamed("list.schema.json", ListSchema)
var compiledRecord = mustCompileNamed("record.schema.json", RecordSchema)
var compiledWrite = mustCompileNamed("write.schema.json", WriteSchema)

// ValidateList validates list output. Takes any to avoid an import cycle.
func ValidateList(items any) error { return validateAgainst(compiledList, items) }

// ValidateRecord validates a read Record. Takes any to avoid an import cycle.
func ValidateRecord(rec any) error { return validateAgainst(compiledRecord, rec) }

// ValidateWrite validates a WriteResult. Takes any to avoid an import cycle.
func ValidateWrite(w any) error { return validateAgainst(compiledWrite, w) }
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/schema/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/schema
git commit -m "feat(schema): gates for list, record, and write-result outputs"
```

---

### Task 6: CLI verbs read / list / new / set

**Files:**
- Modify: `cmd/tfq/main.go`
- Modify: `VOCABULARY.md` (move write ops out of "reserved")
- Test: `cmd/tfq/main_test.go` (add cases)

**Interfaces:**
- New verbs (flat):
  - `read <ref> <dir> [--raw]` → Record JSON, or with `--raw` the body text only.
  - `list <dir> [--status S] [--tag T] [--type T]` → ListItem array.
  - `new <slug> <dir> [--template note|task] [--field k=v ...]` → WriteResult (default template `note`).
  - `set <ref> <dir> [--status S] [--add-tag T] [--field k=v]` → WriteResult.
- `new`/`set` use `time.Now()` for the date. `--field` may repeat; collect into a map (last wins). `--add-tag` may repeat.

- [ ] **Step 1: Write the failing tests (add to `cmd/tfq/main_test.go`)**

```go
func TestRunNewAndSetAndListAndRead(t *testing.T) {
	dir := t.TempDir()

	// new task
	out, code := run([]string{"new", "do-thing", dir, "--template", "task"})
	if code != 0 {
		t.Fatalf("new exit %d: %s", code, out)
	}
	if !contains(out, "\"action\": \"created\"") {
		t.Errorf("new output: %s", out)
	}

	// list shows it as pending
	out, code = run([]string{"list", dir, "--status", "pending"})
	if code != 0 {
		t.Fatalf("list exit %d: %s", code, out)
	}
	if !contains(out, "do-thing") {
		t.Errorf("list output: %s", out)
	}

	// set it to completed
	out, code = run([]string{"set", "do-thing", dir, "--status", "completed"})
	if code != 0 {
		t.Fatalf("set exit %d: %s", code, out)
	}
	if !contains(out, "\"action\": \"updated\"") {
		t.Errorf("set output: %s", out)
	}

	// now no pending tasks
	out, _ = run([]string{"list", dir, "--status", "pending"})
	if contains(out, "do-thing") {
		t.Errorf("task should no longer be pending: %s", out)
	}

	// read --raw shows the body
	out, code = run([]string{"read", "do-thing", dir, "--raw"})
	if code != 0 {
		t.Fatalf("read exit %d: %s", code, out)
	}
	if !contains(out, "do thing") {
		t.Errorf("read --raw output: %s", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/tfq/... -run TestRunNewAndSet`
Expected: FAIL (unknown verbs → exit 2).

- [ ] **Step 3: Add verbs to `run` in `cmd/tfq/main.go`**

Add imports: `"time"`, `"tfq/internal/layout"`, `"tfq/internal/query"`, `"tfq/internal/store"`. Add these cases before `default`:

```go
	case "read":
		pos, flags, err := partition(rest, map[string]bool{"raw": true})
		if err != nil || len(pos) != 2 {
			return usage(), 2
		}
		rec, rerr := query.Read(pos[1], pos[0])
		if rerr != nil {
			return errln(rerr), 1
		}
		if flags["raw"] == "true" {
			return rec.Body, 0
		}
		return mustJSON(rec), 0
	case "list":
		pos, flags, err := partition(rest, nil)
		if err != nil || len(pos) != 1 {
			return usage(), 2
		}
		items, lerr := query.List(pos[0], query.ListFilters{Status: flags["status"], Tag: flags["tag"], Type: flags["type"]})
		if lerr != nil {
			return errln(lerr), 1
		}
		return mustJSON(items), 0
	case "new":
		pos, flags, multi, err := partitionMulti(rest, nil)
		if err != nil || len(pos) != 2 {
			return usage(), 2
		}
		tmpl := layout.Template(flags["template"])
		if tmpl == "" {
			tmpl = layout.TemplateNote
		}
		res, nerr := store.New(pos[1], tmpl, pos[0], multi["field"], time.Now(), layout.DefaultConfig())
		if nerr != nil {
			return errln(nerr), 1
		}
		return mustJSON(res), 0
	case "set":
		pos, flags, multi, err := partitionMulti(rest, nil)
		if err != nil || len(pos) != 2 {
			return usage(), 2
		}
		changes := multi["field"]
		if changes == nil {
			changes = map[string]string{}
		}
		if s, ok := flags["status"]; ok {
			changes["status"] = s
		}
		var addTags []string
		for _, kv := range multi["add-tag-list"] {
			addTags = append(addTags, kv)
		}
		res, serr := store.Set(pos[1], pos[0], changes, addTags)
		if serr != nil {
			return errln(serr), 1
		}
		return mustJSON(res), 0
```

This needs a multi-value parser (for repeated `--field k=v` and `--add-tag`). Add to `cmd/tfq/args.go`:

```go
// partitionMulti is like partition but also collects repeated --field k=v into
// multi["field"] (a map) and repeated --add-tag values into multi["add-tag-list"]
// (a slice, stored as a map's values is awkward, so it is returned via the slice
// map keyed "add-tag-list").
func partitionMulti(raw []string, bools map[string]bool) (pos []string, flags map[string]string, multi map[string]map[string]string, err error) {
	flags = map[string]string{}
	multi = map[string]map[string]string{"field": {}}
	tags := []string{}
	i := 0
	for ; i < len(raw); i++ {
		a := raw[i]
		if !strings.HasPrefix(a, "--") {
			pos = append(pos, a)
			continue
		}
		name := a[2:]
		val := ""
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			name, val = name[:eq], name[eq+1:]
		} else if bools != nil && bools[name] {
			flags[name] = "true"
			continue
		} else {
			if i+1 >= len(raw) {
				return nil, nil, nil, fmt.Errorf("flag --%s needs a value", name)
			}
			i++
			val = raw[i]
		}
		switch name {
		case "field":
			eq := strings.IndexByte(val, '=')
			if eq < 0 {
				return nil, nil, nil, fmt.Errorf("--field needs k=v, got %q", val)
			}
			multi["field"][val[:eq]] = val[eq+1:]
		case "add-tag":
			tags = append(tags, val)
		default:
			flags[name] = val
		}
	}
	multi["add-tag-list"] = map[string]string{}
	for idx, tg := range tags {
		multi["add-tag-list"][fmt.Sprintf("%d", idx)] = tg
	}
	return pos, flags, multi, nil
}
```

> Implementer note: in the `set` case, iterate `multi["add-tag-list"]` map values to build `addTags` (order not significant for tags). Keep the helper's behavior matched to how `run` reads it.

Extend `usage()` to document `read`/`list`/`new`/`set` and drop the "reserved" line.

- [ ] **Step 4: Run tests + build + smoke**

Run: `go test ./cmd/tfq/...`
Expected: PASS.

Run: `go build -o tfq ./cmd/tfq && D=$(mktemp -d) && ./tfq new my-note "$D" && ./tfq list "$D" && ./tfq set my-note "$D" --add-tag done && cat "$D"/*/*/*.md`
Expected: created result JSON; list with the note; updated result; file shows the appended tag.

- [ ] **Step 5: Update `VOCABULARY.md`**

Move `read`/`list`/`new`/`set` into the main verb table with their args/flags, and delete the "Reserved (write ops…)" section (leave a note that the agent-resources fold-in remains future work).

- [ ] **Step 6: Final full-suite run and commit**

Run: `go vet ./... && go test ./...`
Expected: all packages PASS.

```bash
git add cmd/tfq VOCABULARY.md
git commit -m "feat(cmd): read, list, new, set verbs (tfq now read+write)"
```

---

## Self-Review

- **Scope:** `read`/`list`/`new`/`set` delivered; path policy centralized in `layout` as a config seam per the directive; write ops preserve body + key order; every new output mode (list/record/write) schema-gated. ✓
- **Placeholders:** none — runnable code + expected output throughout. The `set` add-tag iteration note is the only prose instruction; the helper and call site are both specified.
- **Type consistency:** `layout.{Config,Rule,Template,DefaultConfig,RelPath,NextSequence}`, `query.{List,ListItem,ListFilters,Read,Record}`, `store.{New,Set,WriteResult}`, `schema.{ValidateList,ValidateRecord,ValidateWrite}`, and the CLI `partition`/`partitionMulti` are used identically across producing/consuming tasks.
- **Deferred:** agent-resources fold-in (4c) — rewrite skills/scripts/doctor to call `tfq`, drop `ov`/`taskmd`/`cue`, file a report. Not in this plan.
