# Structural narrowing + type discoverability — Implementation Plan

> REQUIRED SUB-SKILL: superpowers:executing-plans. TDD, one commit per task,
> `go vet ./... && go test ./...` green after each.
> Spec: `docs/superpowers/specs/2026-06-23-tfq-structural-narrowing-design.md`.

**Goal:** `--in heading|tag|link` narrowing + hit kind labels; `--types` index;
`--new` writes `type:`; docs folded into a brief README (delete HANDOFF/VOCAB).

**Constraints:** no new dependency; `--json` shapes only extended additively;
record-per-file model unchanged.

---

### Task 13: search `--in` classification + `Kinds` + schema

**Files:** `internal/search/search.go`, `internal/search/search_test.go`,
`internal/schema/hits.schema.json`.

- `Hit` gains `Kinds []string \`json:"kinds,omitempty"\``.
- `Filters` gains `In []string`.
- `Search`: always inspect each matched file (reuse the existing `inspect`
  cache); classify each hit; keep iff `In` empty or `kinds ∩ In ≠ ∅`.
- Compile the query once: `qre` = `regexp.Compile(query)` (prefix `(?i)` when
  `IgnoreCase`); nil on error → substring fallback.

Classification helper:

```go
func classify(v model.FileVitals, line int, q *regexp.Regexp, raw string, ic bool) []string {
	kinds := []string{}
	for _, h := range v.Headings {
		if h.Line == line {
			kinds = append(kinds, "heading")
			break
		}
	}
	for _, m := range v.Markers {
		if (m.Kind == model.MarkerHashtag || m.Kind == model.MarkerOrgTag) && m.Line == line && matchVal(m.Value, q, raw, ic) {
			kinds = append(kinds, "tag")
			break
		}
	}
	for _, l := range v.Links {
		label := ""
		if l.Label != nil {
			label = *l.Label
		}
		if l.Line == line && (matchVal(l.Target, q, raw, ic) || matchVal(label, q, raw, ic)) {
			kinds = append(kinds, "link")
			break
		}
	}
	return kinds
}

func matchVal(val string, q *regexp.Regexp, raw string, ic bool) bool {
	if q != nil {
		return q.MatchString(val)
	}
	if ic {
		return strings.Contains(strings.ToLower(val), strings.ToLower(raw))
	}
	return strings.Contains(val, raw)
}
```

Wire it in the hit loop (always inspect; attach kinds; apply `In`):

```go
v, ok := inspect(abs) // now unconditional
if !ok || !passesFilters(v, f) {
	continue
}
kinds := classify(v, ev.Data.LineNumber, qre, query, f.IgnoreCase)
if len(f.In) > 0 && !anyIn(kinds, f.In) {
	continue
}
... Hit{..., Kinds: kinds}
```

`anyIn(have, want []string) bool` returns true if any want ∈ have.

**Tests (TDD):**
- `--in heading`: query matching a heading line is kept; a body-only match is
  dropped.
- `--in tag`: `#battery` line kept for query `battery`; prose `battery` dropped.
- `--in link`: `[[battery-spec]]` line kept; prose dropped.
- `Kinds` populated: a heading hit has `Kinds == ["heading"]`; a prose hit has
  empty `Kinds`.

**Schema:** add to `hits.schema.json` `items.properties`:

```json
"kinds": { "type": "array", "items": { "enum": ["heading", "tag", "link"] } }
```

Steps: RED (new tests) → impl → GREEN → `git commit -m "feat(search): --in heading|tag|link narrowing + hit kinds"`.

---

### Task 14: `query.Types` + schema gate

**Files:** `internal/query/query.go`, `internal/query/query_test.go`,
`internal/schema/types.schema.json`, `internal/schema/schema.go`,
`internal/schema/types_schema_test.go`.

- `TypeCount{Type string \`json:"type"\`; Count int \`json:"count"\`}`.
- `Types(root) ([]TypeCount, error)` — count distinct frontmatter `type:` values,
  sort count-desc then name-asc (parallel to `Tags`, using `fmStr(fm,"type")`,
  skip empty).
- `types.schema.json` (mirror `tags.schema.json`, field `type`) + embed +
  `compiledTypes` + `ValidateTypes`.

**Tests:** `Types` counts/sort; `ValidateTypes` accepts valid, rejects missing
`count`.

Commit: `feat(query): Types index + schema gate`.

---

### Task 15: `--new`/`--task` write `type:`

**Files:** `internal/store/store.go`, `internal/store/new_test.go`.

In `scaffold`:
- task: `base["type"] = "task"`; `order = []string{"type", "id", "title", "status", "priority"}`.
- note: `base["type"] = "note"`; `order = []string{"type", "date", "author", "slug"}`.

(`fields` still override base, so an explicit `type` wins.)

**Tests:** `New(...TemplateTask...)` output contains `type: task`;
`TemplateNote` contains `type: note`; a `fields["type"]="log"` override yields
`type: log`.

Commit: `feat(store): templates write a type: field`.

---

### Task 16: parser — `--in` + `--types`

**Files:** `cmd/tfq/parse.go`, `cmd/tfq/parse_test.go`.

- `Invocation.In []string`; `Mode` adds `ModeTypes`.
- `--types` → `setMode(ModeTypes, ...)`.
- `--in` value flag, validate `heading|tag|link`, append to `inv.In`:

```go
case "in":
	v, err := needVal()
	if err != nil { return inv, err }
	switch v {
	case "heading", "tag", "link":
		inv.In = append(inv.In, v)
	default:
		return inv, usageErr("--in must be heading|tag|link, got " + v)
	}
```

**Tests:** `--in heading --in tag` → `In == [heading tag]`; bad `--in` errors;
`--types` → `ModeTypes`.

Commit: `feat(cmd): --in and --types parsing`.

---

### Task 17: formatters — hit kind labels + types index

**Files:** `cmd/tfq/format.go`, `cmd/tfq/format_test.go`.

- `formatHits`: append a dim ` [kind,…]` when `len(h.Kinds) > 0` (both heading
  and no-heading branches).

```go
func kindTag(kinds []string, p palette) string {
	if len(kinds) == 0 {
		return ""
	}
	return " " + p.dim("["+strings.Join(kinds, ",")+"]")
}
```

- Extract a shared count-index renderer and use it for both tags and types:

```go
type countPair struct {
	name  string
	count int
}

func formatIndex(header string, pairs []countPair, p palette) string {
	if len(pairs) == 0 {
		return ""
	}
	w := 0
	for _, c := range pairs {
		if len(c.name) > w {
			w = len(c.name)
		}
	}
	lines := make([]string, len(pairs))
	for i, c := range pairs {
		lines[i] = fmt.Sprintf("  %s%s  %d", p.tag(c.name), strings.Repeat(" ", w-len(c.name)), c.count)
	}
	return p.bold(header) + "\n" + strings.Join(lines, "\n")
}

func formatTagsIndex(tags []query.TagCount, p palette) string {
	pairs := make([]countPair, len(tags))
	for i, t := range tags {
		pairs[i] = countPair{t.Tag, t.Count}
	}
	return formatIndex("# tags", pairs, p)
}

func formatTypesIndex(types []query.TypeCount, p palette) string {
	pairs := make([]countPair, len(types))
	for i, t := range types {
		pairs[i] = countPair{t.Type, t.Count}
	}
	return formatIndex("# types", pairs, p)
}
```

**Tests:** `formatHits` with a `Kinds:["heading"]` hit contains `[heading]`;
`formatTypesIndex` renders name+count.

Commit: `feat(cmd): hit kind labels + types index formatter`.

---

### Task 18: dispatch — `--types`, thread `--in`, `--new` type, help

**Files:** `cmd/tfq/main.go`, `cmd/tfq/main_test.go`.

- Search: pass `In: inv.In` into `search.Filters`.
- New mode: `tmpl = task` only when `inv.Type == "task"`; set
  `fields["type"] = inv.Type` when `inv.Type != ""`.
- Add `case ModeTypes:` → `query.Types(root)` → `mustJSON` / `formatTypesIndex`.
- `usage()`: clarify `--type` ("match frontmatter type:"), add `--in
  heading|tag|link`, `--types`.

**Tests (e2e):**
- `--root T "battery" --in heading` returns only the heading file.
- `--new x --type task` then `--root T --type task` (filter) finds `x`
  (proves the `type:` write + filter loop).
- `--types` lists `task`.

Commit: `feat(cmd): wire --in/--types, --new type:, help`.

---

### Task 19: docs — brief README, delete HANDOFF + VOCABULARY

**Files:** create `README.md`; `git rm HANDOFF.md VOCABULARY.md`.

`README.md` (brief; no wall of text) covers:
1. One-line what/why.
2. Build / test / run.
3. CLI: the `tfq [OPTIONS] [SELECTOR...]` model — a compact modes/filters/search
   table incl. `--in`, `--types`, `--root`, `--color`, `--json`.
4. "Working in the codebase" for an agent: the record=file mental model, the
   four extractors + package map (one line each), invariants, and a pointer to
   `docs/superpowers/specs/` + `plans/` for depth.

Verify: `go vet ./... && go test ./... && make build && ./tfq --help`; grep that
no doc references `HANDOFF.md`/`VOCABULARY.md`.

Commit: `docs: brief README; remove HANDOFF/VOCABULARY (folded in)`.
