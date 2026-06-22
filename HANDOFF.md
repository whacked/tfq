# tfq — Engineering Handoff

> Audience: a senior/staff engineer picking this up cold. This is the single
> doc to read first. It covers *why* the project exists, the constraints that
> shaped it, the architecture, what is built and verified, and what remains.

---

## 1. TL;DR

`tfq` ("text-file query") is a single Go binary that treats a directory of
frontmatter'd text files (Markdown, Org, …) as **records that form a typed
graph**, and exposes read + write + validate operations over them. It is built
to replace the durable capabilities of two bespoke CLIs used by the
`agent-resources` skills bundle — **`ov`** (Obsidian vault) and **`taskmd`**
(markdown task tracker) — plus the `cue` validation dependency.

- **Language:** Go 1.25. One static binary. No runtime services, no search index.
- **Status:** functionally complete as a standalone read+write tool. All work
  through "write ops" is merged to `main`. A `version` verb sits on an unmerged
  `feat/version` branch.
- **Tests:** TDD throughout; 14 packages green; every output mode is validated
  against a JSON Schema *in the test suite*.

Build & run:

```bash
make build          # injects version via -ldflags; produces ./tfq
make test           # go test ./...
./tfq help          # the full verb surface
./tfq version       # yyyymmdd.<nth-commit-of-day>.<short-hash>
```

The verb surface (all query output is JSON):

```
inspect · read · search · list · links · backlinks · graph · next
new · set · validate · version · help
```

See `VOCABULARY.md` for the authoritative per-verb reference.

---

## 2. Motivation & Context

`agent-resources` (symlinked at `external/agent-resources`, real path a sibling
repo) is a cross-harness **skills bundle** whose skills shell out to external
CLIs: `cue`, `taskmd`, `jq`, `rg`, and optionally `cpd`, `ov`, `ck`. Two of
those — `ov` and `taskmd` — are **bespoke compiled binaries we maintain** that
duplicate a large amount of machinery.

The key observation that motivates the whole project:

> **`ov` and `taskmd` are the same tool wearing two costumes.** Both manage
> *text files with YAML frontmatter that form a typed graph.*

|                | `ov` (notes)                 | `taskmd` (tasks)                       |
|----------------|------------------------------|----------------------------------------|
| Unit           | `.md` + frontmatter          | `.md` + frontmatter                    |
| Edges          | `[[wiki-links]]` in the body | `dependencies:` / `parent:` frontmatter|
| Killer feature | `backlinks` / `graph`        | `next` (blocking-aware ready set)      |
| Search         | full-text (Tantivy index)    | full-text                              |
| Validation     | CUE-in-template              | `taskmd validate`                      |

Both reduce to: a **record** (path + frontmatter + body), **edges** (typed
references between records), and a **collection** (a directory + a schema + an
ID convention). That is literally a *text-file query tool*. Everything else —
the Tantivy index, the kanban board, two separate binaries — is incidental and
was dropped.

The only thing that genuinely needs structured parsing is **frontmatter**.
Bodies are handled by pattern matching. Search needs no index: `rg` over
plaintext is fast enough and removes `ov`'s biggest footgun ("remember to
rebuild the index before searching").

---

## 3. Requirements & Design Decisions

These were settled with the project owner up front and are load-bearing. Treat
them as invariants unless explicitly revisited.

| Decision | Choice | Rationale |
|---|---|---|
| Runtime | **Go** | Static binary; straight path to embed `cuelang.org/go` and JSON-Schema libs *as libraries* (not shelled-out tools). |
| Search | **ripgrep only, no index** | Kills index-staleness; plaintext speed suffices. |
| Validation default | **Liberal** (valid YAML + required fields + edges resolvable; dangling = warning) | Conserve functionality; never fail extraction. |
| Strict validation | **Opt-in, embedded** | `--strict` promotes findings to errors; uses embedded CUE, no `cue` on PATH. |
| Input schema | **Restricted CUE** (`.tfq.cue`) | We evaluate *full* CUE via the embedded lib; the "restriction" is what authors are expected to write, not a parser limit. `@edge` attributes declare graph edges. |
| Output contract | **JSON Schema**, gated in tests | Output *is* JSON; mature Go validators; a mode without a passing schema test is not "done". |
| Formats | **Multi-format by extension** | Not Markdown-only; `.md`/`.org` seeded, unknown handled liberally. |
| Regex dialect | **RE2 only** | The shared dialect of Go `regexp`, ripgrep, *and* CUE's `=~`. One dialect everywhere; extractors are ripgrep-portable. |
| Semantic search | **Out, permanently** | A separate complete program (embeddings) owns it. `ck` stays separate; `cpd` untouched. |
| CLI grammar | **Flat verbs** | Truest to the unified "everything is a record" model; type comes from schema/template, not the command. |
| Vocabulary timing | **Deferred until core solid** | Built read-only core first, finalized verbs later. |
| Path/sharding/ID policy | **Centralized config seam** (`internal/layout`) | Owner directive: "make the path config logic high up so it's a global config" for future user-supplied rules. Defaults replicate agent-resources conventions. |
| Writes | **Preserve body + key order** | `set` uses a `yaml.Node` round-trip, never a map re-serialize. |

---

## 4. Architecture

### Mental model

A **collection** is a directory. Each file whose extension maps to a known
format becomes a **record** = `{path, format, frontmatter, headings, links,
markers}`. Edges come from body links (`[[wiki]]`, markdown, org, …) and
configurable frontmatter fields (`dependencies`, `parent`, …). A **graph** is
built by resolving edge targets against a **multi-key index** of all records.

### Package map (`internal/…` unless noted)

| Package | Responsibility | Notable types/functions |
|---|---|---|
| `model` | Pure data: the output contract | `FileVitals`, `Link`, `Marker`, `Heading`, `Warning` |
| `extract` | Four modular RE2 extractors (never fail) | `Frontmatter`, `Headings`, `Markers`, `Links`, `lineCol` |
| `registry` | extension → format | `FormatFor` |
| `engine` | Orchestrates extractors over one file | `Inspect(path)`, `InspectContent(path, content)` |
| `scan` | Walk a dir → `[]FileVitals` | `Collect(root)` |
| `graph` | Multi-key node index + edge resolution + traversals | `Build`, `Resolve`, `Backlinks`, `Forward`, `Next`, `Edges` |
| `search` | ripgrep wrapper + frontmatter post-filter | `Search(root, query, Filters)` |
| `cueschema` | Load `.tfq.cue`, extract `@edge`, validate frontmatter | `Load`, `Find`, `EdgeFields`, `Validate` |
| `validate` | Assemble a liberal/strict `Report` over a dir | `Run(root, strict)` |
| `layout` | **Path/sharding/ID config seam** | `Config`, `DefaultConfig`, `RelPath`, `NextSequence` |
| `query` | Read-only `List` (filtered) + `Read` (ref→record+body) | `List`, `Read` |
| `store` | Writes: create + frontmatter mutation | `New`, `Set`, `WriteResult` |
| `schema` | Embedded JSON Schemas + validators (test gate) | `Validate*` (one per mode) |
| `cmd/tfq` | The CLI: flat-verb dispatch + flags-anywhere parser | `run`, `partition`, `partitionMulti`, `version` |

### Data flow

```
file(s) ──scan/engine──▶ []FileVitals ──graph.Build──▶ Graph
                              │                          │
                              ├── search (rg) ──▶ Hits   ├── Backlinks / Forward
                              ├── query.List ──▶ ListItem├── Next (blocking ready set)
                              └── query.Read ──▶ Record  └── Edges
                                                          
.tfq.cue ──cueschema.Load──▶ Schema ──┬── Validate(frontmatter) ─┐
                                       └── EdgeFields() ──▶ graph  ├─▶ validate.Report
store.New/Set ──layout.Config──▶ writes (preserve body+order)
```

Every leaf output (`FileVitals`, `Edges`, `Hits`, `Report`, `ListItem[]`,
`Record`, `WriteResult`) has a JSON Schema in `internal/schema/*.schema.json`
that is asserted against real fixture output in `internal/schema/*_test.go`.

---

## 5. The Architectural Keystone: extractors as RE2 regex-sets

> Every extractor is a named **RE2-dialect** regex-set + a small Go
> post-processing step.

RE2 is the dialect of Go `regexp`, ripgrep's engine, *and* CUE's `=~`. One
pattern source therefore runs in-process now and can be delegated to `rg` for
bulk corpus extraction later, with no drift. This is why "delegate to ripgrep"
is free, and why the link parser (the piece expected to keep evolving) is the
most aggressively modularized: a versioned regex-set with a **conformance
corpus** (`internal/extract/testdata/links_corpus.md`) so changes can't silently
regress.

Extractors are **liberal**: they never return a fatal error; they return what
they parsed plus `[]model.Warning`. Only true I/O errors propagate from
`engine.Inspect`.

---

## 6. Schema & Validation Model

- A collection may carry a **`.tfq.cue`** file (discovered by walking up from the
  target dir, like `.taskmd.yaml`).
- It is **real CUE**, evaluated in-process via the embedded `cuelang.org/go`.
  Enums, regex constraints (`=~`), conjunctions (`string & =~…`), list types —
  all work (verified against the agent-resources `reports` schema in
  `internal/cueschema/richschema_test.go`).
- Fields tagged with **`@edge()`** / **`@edge(blocking)`** declare which
  frontmatter fields are graph edges. `cueschema.EdgeFields()` feeds these into
  `graph.Options` (the CLI/`validate` does the translation so `graph` stays
  CUE-free).
- `tfq validate <dir>`: **liberal** (findings are `warning`, exit 0).
  `--strict`: findings are `error`, exit 1 if any. Findings = schema violations
  per record + dangling-edge findings from the graph.

CUE attribute syntax requires parentheses: `@edge()` not `@edge`.

---

## 7. Path/Layout Config Seam (`internal/layout`)

Per owner directive, **all** sharding/ID/filename logic lives here — nothing is
scattered through call sites. `layout.Config` is a plain struct designed to be
loaded from user config in the future; `DefaultConfig()` encodes today's
agent-resources-compatible conventions:

- note → `{yyyy}/{mm}/{yyyy}-{mm}-{dd}.{nnn}-{slug}.md` (**daily** sequence)
- task → `{yyyy}/{mm}/{nnn}-{slug}.md` (**global** sequence)

`RelPath` does token substitution; `NextSequence` computes the next `nnn` by
scanning existing files. All create/path functions take an explicit `date` so
they are deterministic (only the CLI calls `time.Now()`).

To add user-supplied rules later: extend `Config` to be unmarshaled from a
config file (likely a `.tfq.yaml`, distinct from the validation `.tfq.cue`) and
thread it through `store.New`. No call sites should need to change.

---

## 8. Methodology (how the work was done)

Each phase followed the same loop, and a new contributor should continue it:

1. **Brainstorm / decide** the load-bearing forks with the owner (recorded as a
   design spec for the macro decisions).
2. **Write a plan** to `docs/superpowers/plans/…` — bite-sized TDD tasks with
   exact files, complete code, and expected test output.
3. **Execute TDD**: write failing test → confirm RED → minimal impl → GREEN →
   commit. One commit per task.
4. **Schema-gate** every new output mode in tests.
5. **Finish the branch** (merge to `main` after `go vet ./... && go test ./...`).

Risky/uncertain APIs were **spiked first** (e.g. a throwaway test pinned the
`cuelang.org/go` API — `CompileString`, `Unify(...).Validate(cue.Concrete(true))`,
attribute extraction via `Fields(cue.Optional(true))` + trim trailing `?`,
errors via `cueerrors.Errors`).

---

## 9. Results — What Is Built & Verified

Built across five phases, all merged to `main` (plus `version` on a branch):

| Phase | Delivered |
|---|---|
| 1 Extraction core | `model`, four RE2 extractors, `registry`, `engine.Inspect`, `FileVitals` gate |
| 2 Corpus ops | `scan`, multi-key `graph` (backlinks/forward/`next`), ripgrep `search` |
| 3 Validation | `.tfq.cue` via embedded CUE, liberal/strict `validate` Report, `@edge`→graph |
| 4a Vocabulary | flat-verb CLI, flags-anywhere parser, `VOCABULARY.md` |
| 4b Write ops | `layout` seam, `query` (read/list), `store` (new/set, body-preserving) |
| (branch) | `version` verb + `Makefile` build-time injection |

**Verified end-to-end** (not just unit-green): a real dependency-aware task
workflow through the CLI — `new` two tasks, `set` a blocking dependency, `next`
correctly gates the dependent task until the blocker is `completed`, `validate`
passes against a `.tfq.cue`, `list --status pending` reflects state. Two genuine
bugs were caught by end-to-end verification and fixed with regression tests:
task-slug resolution (`001-do-thing.md` now resolves as `do-thing`), and
zero-padded ids being coerced to ints by YAML (now quoted).

---

## 10. What's Deferred / Next

### The big one: fold `tfq` into `agent-resources`

This is the remaining strategic piece and it **edits the live external repo**
(`external/agent-resources`). Per that repo's `AGENTS.md`, an architectural
change there **requires a report** in its `artifacts/reports/`. Scope:

- Rewrite the `ov`, `taskmd`, `notes`, `synthesize`, `doctor` **SKILL.md** docs
  to call `tfq` verbs instead of `ov`/`taskmd`.
- Replace `skills/notes/scripts/new-task.sh` (uses `taskmd add`) with a `tfq new
  --template task` wrapper; replace `validate-note.sh` (uses `cue vet`) with
  `tfq validate`.
- Update `skills/doctor/scripts/check.sh` — it **hardcodes** `for bin in ov
  taskmd ck` and ov-index/taskmd-config checks. There is **no `dependencies.json`**
  despite the README (it was never created); the binary list lives only in
  `check.sh`.
- Leave `ck` (semantic) and `cpd` (structured data) untouched — they are out of
  scope by design.

Before doing this: confirm with the owner, and write the report. `tfq` is now
functionally sufficient to back it (read/search/list/links/backlinks/graph/next/
new/set/validate all exist).

### Smaller deferred items (each is a clean, isolated improvement)

- **`next` multi-field blocking.** `next` currently uses the single default dep
  field `dependencies`. The schema can mark several fields `@edge(blocking)`;
  `Next` should honor all blocking edge fields (plumb `EdgeFields` → `NextOptions`).
- **`--field` list values.** `new`/`set --field k=v` only sets scalar strings.
  Setting a list (e.g. `dependencies`) via CLI isn't supported; `set` currently
  writes a scalar there (it still resolves because `edgeValues` treats a scalar
  as a 1-element list, but it's not ideal). Add list syntax.
- **CUE error message dedup.** A single bad enum expands to ~6 "conflicting
  values" findings plus a disjunction summary. Correct but noisy; collapse to one
  finding per field.
- **Human output format.** Everything is JSON. A `--format human` for `list`/
  `next` would help interactive use (deliberately skipped — `tfq` is agent-first).
- **`version` branch.** `feat/version` is unmerged; merge when ready.

---

## 11. Gotchas & Non-Obvious Decisions

- **Frontmatter line preservation.** `extract.Frontmatter` returns the body with
  the frontmatter block **blanked to empty lines** (not removed), so downstream
  extractor positions stay absolute to the original file.
- **Multi-key resolution.** A record is indexed by path, basename, basename with
  a leading `NNN-` stripped (task slugs), and frontmatter `id`/`slug`/`title`.
  An edge/ref resolves against any key; first-writer-wins on collisions; dangling
  is a warning, never an error.
- **No import cycles in `schema`.** `ValidateEdges/Hits/Report/List/Record/Write`
  take `any` (not the concrete types) so `schema` never imports `graph`/`search`/
  `validate`/`query`/`store`. The *test* files import them; production code does not.
- **Determinism in tests.** Date- and sequence-dependent code takes an explicit
  `date time.Time`; tests pass a fixed date and use `t.TempDir()`. Avoid
  `time.Now()` outside `cmd`.
- **`rg` exit codes.** `search` treats `rg` exit 1 (no matches) as success
  (empty hits); exit ≥2 is a real error.
- **Version is build-time.** `cmd/tfq` has `var version = "dev"`; the `Makefile`
  derives `yyyymmdd.<nth-commit-of-day>.<short-hash>` from git and injects via
  `-ldflags -X main.version=...`. Plain `go build` yields `dev`. Don't move this
  to runtime — a distributed binary shouldn't shell out to git.

---

## 12. File & Doc Index

- **`VOCABULARY.md`** — authoritative per-verb CLI reference + path policy.
- **`Makefile`** — `build` (version-injected), `version`, `test`, `vet`, `clean`.
- **`docs/superpowers/specs/2026-06-21-tfq-streamlining-design.md`** — the macro design spec.
- **`docs/superpowers/plans/`** — one TDD plan per phase (extraction-core,
  corpus-operations, validation, vocabulary, write-ops). These are the most
  detailed record of *what* each piece does and *why* each test exists.
- **`external/agent-resources/`** — the symlinked repo to fold into (read its
  `README.md`, `AGENTS.md`, and `skills/*/SKILL.md`).
- **Schemas:** `internal/schema/*.schema.json` (the output contracts).
- **Conformance corpus:** `internal/extract/testdata/links_corpus.md`.

---

## 13. Build / Test / Run Quickref

```bash
make build                 # ./tfq, version injected
make test                  # go test ./...
make vet                   # go vet ./...
make version               # print the derived version string

# Inspect one file
./tfq inspect path/to/note.md

# Query a collection (a directory)
./tfq search "term" ./vault --type note --tag urgent
./tfq backlinks some-note ./vault
./tfq graph ./vault
./tfq next ./vault                       # blocking-aware ready tasks
./tfq list ./vault --status pending

# Write
./tfq new my-idea ./vault --template note
./tfq new ship-it ./vault --template task --field priority=high
./tfq set ship-it ./vault --status completed --add-tag reviewed

# Validate against ./vault/.tfq.cue
./tfq validate ./vault          # liberal, exit 0
./tfq validate ./vault --strict # errors, exit 1 on any finding
```

A minimal `.tfq.cue`:

```cue
status:        "pending" | "in-progress" | "completed" | "blocked" | "cancelled"
priority?:     "low" | "medium" | "high" | "critical"
dependencies?: [...string] @edge(blocking)
parent?:       string      @edge()
```
