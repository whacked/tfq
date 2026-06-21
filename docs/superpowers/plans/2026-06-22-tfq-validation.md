# tfq Validation Implementation Plan (Phase 3)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add per-collection schema validation driven by a restricted-CUE `.tfq.cue` file: liberal by default (violations are warnings, exit 0), strict on demand (`--strict`, violations are errors, exit non-zero); `@edge` attributes in the schema declare which frontmatter fields are graph edges, unifying validation with graph traversal.

**Architecture:** A `cueschema` package wraps the embedded `cuelang.org/go` evaluator: it loads a `.tfq.cue` file, validates a frontmatter map (unify + concrete-validate), and extracts `@edge` attributes. A `validate` package assembles a `Report` over a scanned directory — schema findings per record plus graph dangling-edge findings — with severity gated by liberal/strict mode. The `graph` package stays CUE-free (it already takes `Options`); the consumer translates schema edge fields into `graph.Options`. A new `tfq validate` verb emits a schema-gated `Report`.

**Tech Stack:** Go 1.25, `cuelang.org/go v0.16.1` (embedded CUE evaluator), `github.com/santhosh-tekuri/jsonschema/v6` (output gate). Builds on Phase 1/2 packages: `model`, `scan`, `graph`, `schema`.

## Global Constraints

- **Verified cuelang API (from spike, pin to these calls):**
  - `ctx := cuecontext.New()`; `schema := ctx.CompileString(src)`; check `schema.Err()`.
  - validate data: `unified := schema.Unify(ctx.Encode(fm))`; `err := unified.Validate(cue.Concrete(true), cue.All())` (nil = valid). Missing required field (declared without `?`) and bad enum both produce an error; extra fields are tolerated (open struct).
  - edges: `iter, _ := schema.Fields(cue.Optional(true))`; per field `name := strings.TrimSuffix(iter.Selector().String(), "?")`; `attr := iter.Value().Attribute("edge")`; present iff `attr.Err() == nil`; blocking iff `attr.NumArgs() > 0 && first-arg == "blocking"`.
  - `@edge` MUST have parens in CUE: `@edge()` or `@edge(blocking)`.
- **Schema file:** `.tfq.cue` in the collection dir; discovered by walking up from the target directory.
- **Liberal vs strict:** liberal → all findings are `warning` severity, `Report.OK = true`; strict → schema/dangling findings are `error` severity, `Report.OK = false` if any error finding. Exit code mirrors `OK`.
- **No schema present:** validation still runs — only graph dangling-edge findings are produced; `OK` is true in liberal, and in strict `OK` is false only if a dangling edge exists.
- `graph` must not import `cueschema` (keeps the heavy CUE dep out of the graph core). The `validate` package and CLI translate `[]cueschema.EdgeField` → `graph.Options`.
- Output of `validate` is a `Report` validated against a JSON Schema in tests.
- Slices in outputs are non-nil (`[]`).

---

### Task 1: cueschema — load schema + extract edge fields

**Files:**
- Create: `internal/cueschema/cueschema.go`
- Test: `internal/cueschema/cueschema_test.go`
- Test fixture: `internal/cueschema/testdata/.tfq.cue`
- Modify: `go.mod`, `go.sum` (cuelang already added; commit it here)

**Interfaces:**
- Consumes: `cuelang.org/go/cue`, `cuelang.org/go/cue/cuecontext`.
- Produces:
  - `type EdgeField struct { Name string; Blocking bool }`
  - `type Schema struct { ... }` (holds `cue.Context` + compiled `cue.Value`)
  - `func Load(path string) (*Schema, error)` — read + compile a `.tfq.cue`; compile errors are returned.
  - `func Find(startDir string) (string, bool)` — walk up from `startDir` returning the first `.tfq.cue` path found.
  - `func (s *Schema) EdgeFields() []EdgeField` — sorted by Name.

- [ ] **Step 1: Write the fixture**

```text
# internal/cueschema/testdata/.tfq.cue
status:        "pending" | "in-progress" | "completed" | "blocked" | "cancelled"
priority?:     "low" | "medium" | "high" | "critical"
dependencies?: [...string] @edge(blocking)
parent?:       string      @edge()
```

- [ ] **Step 2: Write the failing test**

```go
// internal/cueschema/cueschema_test.go
package cueschema

import (
	"path/filepath"
	"testing"
)

func TestLoadAndEdgeFields(t *testing.T) {
	s, err := Load(filepath.Join("testdata", ".tfq.cue"))
	if err != nil {
		t.Fatal(err)
	}
	efs := s.EdgeFields()
	got := map[string]bool{}
	for _, e := range efs {
		got[e.Name] = e.Blocking
	}
	if b, ok := got["dependencies"]; !ok || !b {
		t.Errorf("dependencies should be a blocking edge: %#v", efs)
	}
	if b, ok := got["parent"]; !ok || b {
		t.Errorf("parent should be a non-blocking edge: %#v", efs)
	}
	if _, ok := got["status"]; ok {
		t.Errorf("status is not an edge field: %#v", efs)
	}
}

func TestFind(t *testing.T) {
	// testdata/ holds .tfq.cue; Find from a nested dir should locate it
	got, ok := Find("testdata")
	if !ok {
		t.Fatal("expected to find .tfq.cue under testdata")
	}
	if filepath.Base(got) != ".tfq.cue" {
		t.Errorf("Find returned %q", got)
	}
}

func TestLoadCompileError(t *testing.T) {
	if _, err := Load("testdata/does-not-exist.cue"); err == nil {
		t.Error("expected error loading missing file")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/cueschema/...`
Expected: FAIL (`Load` etc undefined).

- [ ] **Step 4: Write the implementation**

```go
// internal/cueschema/cueschema.go
package cueschema

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

// EdgeField names a frontmatter field declared as a graph edge via @edge.
type EdgeField struct {
	Name     string
	Blocking bool
}

// Schema is a compiled .tfq.cue collection schema.
type Schema struct {
	ctx   *cue.Context
	value cue.Value
}

// Load compiles a .tfq.cue file.
func Load(path string) (*Schema, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	ctx := cuecontext.New()
	v := ctx.CompileBytes(b)
	if v.Err() != nil {
		return nil, v.Err()
	}
	return &Schema{ctx: ctx, value: v}, nil
}

// Find walks up from startDir returning the first .tfq.cue path found.
func Find(startDir string) (string, bool) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		dir = startDir
	}
	for {
		cand := filepath.Join(dir, ".tfq.cue")
		if _, err := os.Stat(cand); err == nil {
			return cand, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// EdgeFields returns the frontmatter fields marked with @edge, sorted by name.
func (s *Schema) EdgeFields() []EdgeField {
	out := []EdgeField{}
	iter, err := s.value.Fields(cue.Optional(true))
	if err != nil {
		return out
	}
	for iter.Next() {
		name := strings.TrimSuffix(iter.Selector().String(), "?")
		attr := iter.Value().Attribute("edge")
		if attr.Err() != nil {
			continue
		}
		blocking := false
		if attr.NumArgs() > 0 {
			if arg, _ := attr.String(0); arg == "blocking" {
				blocking = true
			}
		}
		out = append(out, EdgeField{Name: name, Blocking: blocking})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/cueschema/...`
Expected: PASS.

- [ ] **Step 6: Tidy and commit**

```bash
go mod tidy
git add go.mod go.sum internal/cueschema
git commit -m "feat(cueschema): load .tfq.cue and extract @edge fields"
```

---

### Task 2: cueschema — validate a frontmatter map

**Files:**
- Modify: `internal/cueschema/cueschema.go` (add `Violation` + `Validate`)
- Test: `internal/cueschema/validate_test.go`

**Interfaces:**
- Produces:
  - `type Violation struct { Field, Message string }`
  - `func (s *Schema) Validate(fm map[string]any) []Violation` — empty slice if valid; one or more violations otherwise. Never panics on odd input.

- [ ] **Step 1: Write the failing test**

```go
// internal/cueschema/validate_test.go
package cueschema

import (
	"path/filepath"
	"testing"
)

func loadTestSchema(t *testing.T) *Schema {
	t.Helper()
	s, err := Load(filepath.Join("testdata", ".tfq.cue"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestValidateValid(t *testing.T) {
	s := loadTestSchema(t)
	if v := s.Validate(map[string]any{"status": "pending"}); len(v) != 0 {
		t.Errorf("valid frontmatter produced violations: %#v", v)
	}
	// extra fields are tolerated
	if v := s.Validate(map[string]any{"status": "completed", "extra": "x"}); len(v) != 0 {
		t.Errorf("extra field should be tolerated: %#v", v)
	}
}

func TestValidateBadEnum(t *testing.T) {
	s := loadTestSchema(t)
	v := s.Validate(map[string]any{"status": "bogus"})
	if len(v) == 0 {
		t.Error("bad enum value should produce a violation")
	}
}

func TestValidateMissingRequired(t *testing.T) {
	s := loadTestSchema(t)
	// status is required (no ?); omit it
	v := s.Validate(map[string]any{"priority": "low"})
	if len(v) == 0 {
		t.Error("missing required status should produce a violation")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cueschema/... -run TestValidate`
Expected: FAIL (`Validate` undefined).

- [ ] **Step 3: Add the implementation to `internal/cueschema/cueschema.go`**

Add the import `cueerrors "cuelang.org/go/cue/errors"` and:

```go
// Violation is a single schema rule that the frontmatter failed.
type Violation struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Validate checks a frontmatter map against the schema. Returns an empty slice
// when valid, or one Violation per failed rule.
func (s *Schema) Validate(fm map[string]any) []Violation {
	out := []Violation{}
	data := s.ctx.Encode(fm)
	if data.Err() != nil {
		return []Violation{{Field: "", Message: "cannot encode frontmatter: " + data.Err().Error()}}
	}
	unified := s.value.Unify(data)
	if err := unified.Validate(cue.Concrete(true), cue.All()); err != nil {
		for _, e := range cueerrors.Errors(err) {
			field := ""
			if p := e.Path(); len(p) > 0 {
				field = strings.Join(p, ".")
			}
			out = append(out, Violation{Field: field, Message: e.Error()})
		}
		if len(out) == 0 {
			out = append(out, Violation{Field: "", Message: err.Error()})
		}
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cueschema/... -run TestValidate`
Expected: PASS.

> Implementer note: if `cueerrors.Errors(err)` or `e.Path()` differs in v0.16.1, fall back to a single `Violation{Field:"", Message: err.Error()}`. The tests only assert `len(v) != 0`, so a coarse single-violation result still passes; refine field attribution only if the API supports it cleanly.

- [ ] **Step 5: Commit**

```bash
git add internal/cueschema/cueschema.go internal/cueschema/validate_test.go
git commit -m "feat(cueschema): validate frontmatter against a .tfq.cue schema"
```

---

### Task 3: validate — assemble a Report over a directory

**Files:**
- Create: `internal/validate/validate.go`
- Test: `internal/validate/validate_test.go`
- Test fixtures: `internal/validate/testdata/vault/.tfq.cue`, `internal/validate/testdata/vault/good.md`, `internal/validate/testdata/vault/bad.md`

**Interfaces:**
- Consumes: `scan.Collect`, `graph.Build/DefaultOptions/Options`, `cueschema.Find/Load/EdgeField`, `model`.
- Produces:
  - `type Finding struct { Path, Field, Message, Severity string }` (`severity` ∈ `error`|`warning`)
  - `type Report struct { Findings []Finding; OK bool }`
  - `func Run(root string, strict bool) (Report, error)` — finds+loads `.tfq.cue` (optional), scans records, validates each, builds the graph with schema edge fields (or defaults) and folds dangling-edge warnings into findings. `OK = false` iff strict and there is at least one finding.

- [ ] **Step 1: Write the fixtures**

```text
# internal/validate/testdata/vault/.tfq.cue
status:        "pending" | "in-progress" | "completed" | "blocked" | "cancelled"
dependencies?: [...string] @edge(blocking)
```

```text
<!-- internal/validate/testdata/vault/good.md -->
---
id: "001"
status: completed
---
# Good
```

```text
<!-- internal/validate/testdata/vault/bad.md -->
---
id: "002"
status: not-a-real-status
dependencies: ["ghost"]
---
# Bad links to [[nowhere]]
```

- [ ] **Step 2: Write the failing test**

```go
// internal/validate/validate_test.go
package validate

import "testing"

func hasFinding(r Report, path, sev string) bool {
	for _, f := range r.Findings {
		if f.Path == path && f.Severity == sev {
			return true
		}
	}
	return false
}

func TestRunLiberal(t *testing.T) {
	r, err := Run("testdata/vault", false)
	if err != nil {
		t.Fatal(err)
	}
	if !r.OK {
		t.Errorf("liberal run should be OK even with findings")
	}
	// bad.md has a bad enum + a dangling dep + a dangling wiki link -> warnings
	if !hasFinding(r, "bad.md", "warning") {
		t.Errorf("expected warnings on bad.md: %#v", r.Findings)
	}
	// good.md is clean
	if hasFinding(r, "good.md", "warning") || hasFinding(r, "good.md", "error") {
		t.Errorf("good.md should have no findings: %#v", r.Findings)
	}
}

func TestRunStrict(t *testing.T) {
	r, err := Run("testdata/vault", true)
	if err != nil {
		t.Fatal(err)
	}
	if r.OK {
		t.Errorf("strict run should fail given bad.md")
	}
	if !hasFinding(r, "bad.md", "error") {
		t.Errorf("expected error severity on bad.md in strict mode: %#v", r.Findings)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/validate/...`
Expected: FAIL (`Run` undefined).

- [ ] **Step 4: Write the implementation**

```go
// internal/validate/validate.go
package validate

import (
	"sort"

	"tfq/internal/cueschema"
	"tfq/internal/graph"
	"tfq/internal/model"
	"tfq/internal/scan"
)

// Finding is one validation result for one record.
type Finding struct {
	Path     string `json:"path"`
	Field    string `json:"field"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
}

// Report is the full validation result over a directory.
type Report struct {
	Findings []Finding `json:"findings"`
	OK       bool      `json:"ok"`
}

// Run validates every record under root against the discovered .tfq.cue (if any)
// and against graph edge resolution. strict promotes findings to errors.
func Run(root string, strict bool) (Report, error) {
	sev := "warning"
	if strict {
		sev = "error"
	}
	findings := []Finding{}

	recs, scanWarns, err := scan.Collect(root)
	if err != nil {
		return Report{}, err
	}
	for _, w := range scanWarns {
		findings = append(findings, Finding{Path: "", Field: "", Message: w.Message, Severity: "warning"})
	}

	// schema (optional)
	opts := graph.DefaultOptions()
	if path, ok := cueschema.Find(root); ok {
		if s, lerr := cueschema.Load(path); lerr == nil {
			for _, r := range recs {
				for _, v := range s.Validate(r.Frontmatter) {
					findings = append(findings, Finding{Path: r.Path, Field: v.Field, Message: v.Message, Severity: sev})
				}
			}
			if efs := s.EdgeFields(); len(efs) > 0 {
				names := make([]string, len(efs))
				for i, e := range efs {
					names[i] = e.Name
				}
				opts = graph.Options{FrontmatterEdgeFields: names}
			}
		} else {
			findings = append(findings, Finding{Message: "schema load error: " + lerr.Error(), Severity: sev})
		}
	}

	// graph dangling edges
	g := graph.Build(recs, opts)
	for _, w := range g.Warnings() {
		findings = append(findings, Finding{Path: pathOf(w), Field: "", Message: w.Message, Severity: sev})
	}

	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Path != findings[j].Path {
			return findings[i].Path < findings[j].Path
		}
		return findings[i].Message < findings[j].Message
	})

	ok := true
	for _, f := range findings {
		if f.Severity == "error" {
			ok = false
		}
	}
	return Report{Findings: findings, OK: ok}, nil
}

// pathOf extracts the leading "path:" prefix from a graph warning message.
func pathOf(w model.Warning) string {
	msg := w.Message
	for i := 0; i < len(msg); i++ {
		if msg[i] == ':' {
			return msg[:i]
		}
	}
	return ""
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/validate/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/validate
git commit -m "feat(validate): liberal/strict report over schema + graph edges"
```

---

### Task 4: Report output JSON Schema + gate test

**Files:**
- Create: `internal/schema/report.schema.json`
- Modify: `internal/schema/schema.go` (embed + validator)
- Test: `internal/schema/report_schema_test.go`

**Interfaces:**
- Produces: `var ReportSchema []byte`, `func ValidateReport(report any) error`.

- [ ] **Step 1: Write the schema**

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://tfq/schemas/report.json",
  "title": "Report",
  "type": "object",
  "additionalProperties": false,
  "required": ["findings", "ok"],
  "properties": {
    "ok": { "type": "boolean" },
    "findings": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["path", "field", "message", "severity"],
        "properties": {
          "path": { "type": "string" },
          "field": { "type": "string" },
          "message": { "type": "string" },
          "severity": { "type": "string", "enum": ["error", "warning"] }
        }
      }
    }
  }
}
```

- [ ] **Step 2: Write the failing test**

```go
// internal/schema/report_schema_test.go
package schema

import (
	"testing"

	"tfq/internal/validate"
)

func TestReportOutputMatchesSchema(t *testing.T) {
	for _, strict := range []bool{false, true} {
		r, err := validate.Run("../validate/testdata/vault", strict)
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidateReport(r); err != nil {
			t.Errorf("strict=%v report schema violation: %v", strict, err)
		}
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/schema/... -run TestReport`
Expected: FAIL (`ValidateReport` undefined).

- [ ] **Step 4: Add to `internal/schema/schema.go`**

```go
//go:embed report.schema.json
var ReportSchema []byte

var compiledReport = mustCompileNamed("report.schema.json", ReportSchema)

// ValidateReport validates a validation Report against the embedded schema.
func ValidateReport(report any) error { return validateAgainst(compiledReport, report) }
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/schema/...`
Expected: PASS (all gates).

- [ ] **Step 6: Commit**

```bash
git add internal/schema
git commit -m "feat(schema): output gate for validation Report"
```

---

### Task 5: CLI `validate` verb + schema-aware graph wiring

**Files:**
- Modify: `cmd/tfq/main.go`
- Test: `cmd/tfq/main_test.go` (add cases)

**Interfaces:**
- Consumes: `validate.Run`, `cueschema.Find/Load`, existing `scan`/`graph`.
- Produces:
  - `tfq validate <dir> [--strict]` → prints `Report` JSON; exit 0 if `OK`, exit 1 otherwise.
  - `buildGraph(dir)` updated: when a `.tfq.cue` exists at/above `dir`, use its `@edge` fields for `graph.Options`; otherwise defaults. (Makes `graph`/`backlinks` honor declared edges.)

- [ ] **Step 1: Write the failing test (add to `cmd/tfq/main_test.go`)**

```go
func TestRunValidate(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, ".tfq.cue", "status: \"pending\" | \"completed\"\n")
	mustWrite(t, dir, "ok.md", "---\nstatus: completed\n---\n# ok\n")

	out, code := run([]string{"validate", dir})
	if code != 0 {
		t.Fatalf("liberal validate should exit 0, got %d: %s", code, out)
	}
	if !contains(out, "\"ok\": true") {
		t.Errorf("expected ok:true: %s", out)
	}

	// strict over a bad record exits 1
	mustWrite(t, dir, "bad.md", "---\nstatus: nope\n---\n# bad\n")
	_, code = run([]string{"validate", dir, "--strict"})
	if code != 1 {
		t.Errorf("strict validate over bad record should exit 1, got %d", code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/tfq/... -run TestRunValidate`
Expected: FAIL (validate is an unknown subcommand → exit 2).

- [ ] **Step 3: Update `cmd/tfq/main.go`**

Add imports `"tfq/internal/cueschema"` and `"tfq/internal/validate"`. Add a `validate` case to the switch (before `default`):

```go
	case "validate":
		if len(args) < 2 || len(args) > 3 {
			return usage(), 2
		}
		strict := false
		dir := args[1]
		if len(args) == 3 {
			if args[2] != "--strict" {
				return usage(), 2
			}
			strict = true
			dir = args[1]
		}
		rep, err := validate.Run(dir, strict)
		if err != nil {
			fmt.Fprintln(os.Stderr, "tfq: "+err.Error())
			return "", 1
		}
		code := 0
		if !rep.OK {
			code = 1
		}
		return mustJSON(rep), code
```

Replace `buildGraph` with the schema-aware version:

```go
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
```

Update `usage()`:

```go
func usage() string {
	return "usage: tfq <inspect <file> | graph <dir> | backlinks <ref> <dir> | next <dir> | search <query> <dir> | validate <dir> [--strict]>"
}
```

- [ ] **Step 4: Run tests + build + smoke**

Run: `go test ./cmd/tfq/...`
Expected: PASS.

Run: `go build -o tfq ./cmd/tfq && ./tfq validate internal/validate/testdata/vault; echo "exit=$?"`
Expected: JSON report with `"ok": true` (liberal default), exit 0.

Run: `./tfq validate internal/validate/testdata/vault --strict; echo "exit=$?"`
Expected: report with `"ok": false`, exit 1.

- [ ] **Step 5: Final full-suite run and commit**

Run: `go vet ./... && go test ./...`
Expected: all packages PASS.

```bash
git add cmd/tfq
git commit -m "feat(cmd): validate verb + schema-aware graph edge wiring"
```

---

## Self-Review

- **Spec coverage (Phase 3):** liberal validation default (Task 3, severity=warning, OK=true) ✓; strict mode (Task 3/5, severity=error, exit 1) ✓; `.tfq.cue` restricted-CUE schema via embedded cuelang (Tasks 1-2) ✓; `@edge` attributes feed the graph (Tasks 1, 3, 5) ✓; per-mode output schema + gate test (Task 4) ✓; schema discovery by walking up (Task 1 `Find`) ✓; graph stays CUE-free, consumer translates edges (constraint honored — `validate` and `cmd` do the translation) ✓.
- **Placeholders:** none — every step has runnable code + expected result. Task 2 carries an explicit fallback note for the cuelang error-introspection API.
- **Type consistency:** `cueschema.{Schema,EdgeField,Violation,Load,Find,Validate,EdgeFields}`, `validate.{Finding,Report,Run}`, `schema.ValidateReport`, and the `cmd` wiring use identical names across producing/consuming tasks. `graph.Options{FrontmatterEdgeFields}` is reused verbatim from Phase 2.
- **Deferred (noted):** `next` honoring multiple `@edge(blocking)` fields (currently uses the default `dependencies` dep field) is a future refinement, not in this plan.
