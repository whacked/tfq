# tfq — Streamlining `ov` + `taskmd` into a Format-Aware Extraction Engine

- **Date:** 2026-06-21
- **Status:** accepted
- **Author:** agent (with directedglaph@gmail.com)
- **Topic:** Replace the bespoke `ov` (Obsidian vault) and `taskmd` (markdown task tracker) CLIs with a single Go tool, `tfq`, that treats text files as frontmatter + a typed graph, optimizing for speed and a small attack surface.

---

## 1. Problem & Context

`agent-resources` is a cross-harness skills bundle whose skills shell out to several external CLIs: `cue`, `taskmd`, `jq`, `rg`, and optionally `cpd`, `ov`, `ck`. Two of these — `ov` and `taskmd` — are bespoke compiled binaries that we maintain and that duplicate a large amount of conceptual machinery.

Observation: **`ov` and `taskmd` are the same tool wearing two costumes.** Both manage *text files with YAML frontmatter that form a typed graph*:

| | `ov` (notes) | `taskmd` (tasks) |
|---|---|---|
| Unit | `.md` + frontmatter | `.md` + frontmatter |
| Edges | `[[wiki-links]]` in the body | `dependencies:` / `parent:` / `context:` in frontmatter |
| Killer feature | `backlinks` / `graph` traversal | `next` (blocking-aware ready set) |
| Search | full-text (Tantivy index) | full-text |
| Validation | CUE-in-template (`validate-frontmatter.sh`) | `taskmd validate` |

Both reduce to: **a record** (path + frontmatter + body), **edges** (typed references between records), and **a collection** (a directory + a schema + an ID convention). That is literally a *text-file query tool*. Everything else — the Tantivy index, the kanban board, two separate binaries — is incidental and is dropped.

The only thing that genuinely needs structured parsing is **frontmatter**. Bodies are handled by pattern matching (`rg` / RE2 regex). Search needs no index: `rg` over plaintext is fast enough and removes the single biggest footgun in `ov` ("remember to rebuild the index before searching").

## 2. Goals

1. One Go binary, `tfq`, replacing the durable capabilities of `ov` and `taskmd`.
2. **Multi-format**, not markdown-only: handle any text file recognized by extension (md, org, txt, and future semi-structured formats).
3. **A hard, predefined, versioned output contract** for every interaction mode, with **schema validation of real outputs enforced in the test suite**.
4. **Modular, liberal, iterable extraction** of frontmatter, links, tags/markers, and headings.
5. Optimize for speed; keep the attack surface small.
6. Liberal-by-default schema validation; strict mode optional and embedded (no hard `cue` dependency).

## 3. Non-Goals (YAGNI — explicitly OUT)

- **No search index / watcher / daemon.** Search is `rg` over plaintext, always.
- **No semantic / embedding search, ever.** A separate, complete program already owns that. The only future hook is *cross-referencing*, and any semantic portion of that is delegated away.
- **No full CUE syntax.** At most a constrained CUE-*shorthand* for input frontmatter.
- **No kanban / board / phase UI** in the core.
- **No vocabulary/CLI-surface design up front.** We stay in code building the library core until it is solid; CLI verbs/flags are deliberately deferred to a later phase.
- **Do not touch `cpd`.** `ck` (semantic) stays a separate optional tool.
- Preserve the supersede-don't-edit-in-place invariant for agent notes — a workflow rule we honor, not logic we build.

## 4. Decisions (resolved forks)

| Fork | Decision | Rationale |
|---|---|---|
| End-state runtime | **Go** | Static binary, strong stdlib for YAML/file-walking, and a straight path to embed `cuelang.org/go` and JSON-Schema libraries *as libraries* rather than shelling out. |
| Search | **`rg` only, no index** | Removes the index-staleness footgun; plaintext speed is sufficient. |
| Validation default | **Liberal** (valid YAML + required fields + edges resolvable; dangling = warning) | Conserves functionality; schemas exist mainly to declare graph edges, not enforce rich types. |
| Input frontmatter schema format | **CUE-shorthand** (a constrained subset, transformable to CUE/JSON-Schema) | CUE's constraint syntax is strong for input; full CUE is too complex. |
| Output contract format | **JSON Schema** | Output *is* JSON; mature, fast Go validators; idiomatic for gating JSON in tests. (CUE can emit JSON Schema later if we want one source.) |
| Presentation/vocabulary | **Deferred** | Build the core library + tests first; design CLI verbs only once the core is solid. |

## 5. Architecture

### 5.1 Front door: extension → format → extractor-set registry

A data-driven registry maps a file extension to a *format* and the set of *extractors* that apply. Extensible by design; `.md`, `.org`, `.txt` are seeded, with "unknown/semi-structured" treated as a first-class case (run the format-agnostic extractors, collect warnings, never fail).

### 5.2 Architectural keystone: extractors are RE2 regex-sets

> **Every extractor is defined as a named RE2-dialect regex-set plus a small Go post-processing step.**

RE2 is the regex dialect of *both* Go's `regexp` package **and** ripgrep's Rust engine (neither supports backreferences or lookaround). One pattern source therefore runs identically:

- **in-process** (Go `regexp`) for single-file precision, and
- **via `rg`** for bulk corpus extraction — same patterns, no drift.

This is what makes "delegate to ripgrep" free, keeps the parsers honest and fast, and lets us iterate patterns in one place.

### 5.3 The four modular extractors

Each is an independent module. Input = content (+ a format hint). Output = typed records **with line/column positions**. An extractor **never fails** — it is liberal and collects warnings instead of erroring.

| Module | Handles (initial → iterate) | Notes |
|---|---|---|
| **Frontmatter** | YAML `---` (later TOML `+++`, Org `#+KEY:`) | the only "real" structured parse; returns a raw map |
| **Links** | markdown `[text](url)`, wiki `[[t]]` / `[[t\|alias]]`, embeds `![[…]]`, Org `[[link][desc]]`, autolinks `<url>`, bare URLs | **highest modularity investment**: versioned regex-set + a golden conformance corpus so iteration is safe |
| **Markers** | `#hashtag`, Org `:tag:tag2:`, `<single angle phrase>`, `<<double angle phrase>>` | one "marker" abstraction, `kind`-tagged |
| **Headings** | markdown `#`/`##`, Org `*`/`**` | level + text + position |

The **link parser** is expected to need ongoing iteration (Org is very liberal; future semi-structured formats are unknown). It is therefore the most aggressively modularized piece: a versioned regex-set governed by a conformance corpus, so additions cannot silently regress the others.

### 5.4 The minimal engine (built on the extractors)

1. **Extract** — walk files; `rg` finds candidate files fast; parse frontmatter only on matches.
2. **Graph** — build an in-memory typed graph from configurable edge fields (body wiki-links + frontmatter references). Traversals: backlinks, forward-links, neighborhood, subtree, and `next` (topological + blocking).
3. **Validate** — liberal default; optional strict via embedded schema lib.
4. **Search** — thin `rg` wrapper; frontmatter filters (`tag:` / `type:` / `date:`) applied by post-filtering hits on parsed frontmatter.
5. **Create / append** — scaffold a record from a schema/template; append under a heading.

## 6. The Output Contract

### 6.1 `FileVitals` (comprehensive query-by-file)

The foundational operation: query a file, get *all* its vitals as schema-valid JSON.

```jsonc
{
  "path": "...", "ext": ".md", "format": "markdown",
  "frontmatter": { /* raw parsed map */ },
  "headings": [ { "level": 1, "text": "...", "line": 3 } ],
  "links":    [ { "kind": "wiki", "target": "...", "label": null, "line": 8, "col": 4 } ],
  "markers":  [ { "kind": "hashtag|org-tag|angle|double-angle", "value": "...", "line": 9, "col": 1 } ],
  "warnings": [ /* liberal: surfaced, never fatal */ ]
}
```

### 6.2 Schema-as-contract, enforced in tests

- **Every interaction mode has a predefined, versioned JSON Schema** for its output: `file-vitals` first; later `search`, `graph`, `validate`, `next`.
- **Schema validation of real outputs is a CI test gate.** A golden fixture corpus is run through the engine, and each emitted document is asserted against its mode's schema on every run. A mode without a passing schema test is not "done."

## 7. Phasing (each phase ships with its schema + tests; small surface)

- **Phase 0 — Contracts.** `FileVitals` JSON Schema + the extension/format registry + a golden fixture set (`.md`, `.org`, an Obsidian-flavored file, and one deliberately weird semi-structured file).
- **Phase 1 — Extraction core ("the core").** The four modular extractors as RE2 regex-sets; single entry `Inspect(path) → FileVitals`; tests validate every output against the schema. Link-parser conformance corpus established here.
- **Phase 2 — Corpus operations.** Search (`rg`) and graph (backlinks / neighborhood / `next`) built *from* extracted edges. Each gets its own output schema + schema-checked tests.
- **Phase 3 — Validation.** Liberal frontmatter/edge validation default; CUE-shorthand → optional embedded strict mode.
- **Phase 4 — Vocabulary & skill integration.** *Only now* design CLI verbs/flags; fold into `agent-resources`; drop `ov` / `taskmd` / `cue` from `dependencies.json`. `ck` / `cpd` untouched.

## 8. Success Criteria

- `tfq` produces a schema-valid `FileVitals` JSON for every fixture in the corpus across all seeded formats.
- All four extractors are independently unit-tested with positions, and the link parser has a standalone conformance corpus.
- Output schema validation runs as a test gate in CI.
- Search returns results via `rg` with no index step.
- Graph traversals (backlinks, `next`) operate correctly on a fixture collection.
- No semantic search, no index, no full-CUE dependency anywhere in the core.

## 9. Module Boundaries (for isolation & testability)

- `registry` — extension → format → extractor-set. *Depends on:* nothing. *Used by:* engine.
- `extract/frontmatter`, `extract/links`, `extract/markers`, `extract/headings` — each: content in, typed records out, never fails. *Depend on:* a shared RE2 pattern-set helper. *Independently testable.*
- `model` — `FileVitals`, `Link`, `Marker`, `Heading`, `Record`, `Edge`. *Pure data.*
- `engine` — `Inspect(path) → FileVitals`; orchestrates registry + extractors. *Depends on:* registry, extractors, model.
- `schema` — embedded JSON Schemas + a validator used by tests (and later by `validate`). *Depends on:* model (shape only).
- `graph` (Phase 2) — builds/traverses the typed graph. *Depends on:* model.
- `search` (Phase 2) — `rg` wrapper + frontmatter post-filter. *Depends on:* model.

Each unit answers: *what does it do, how do you use it, what does it depend on?* — and can be changed internally without breaking consumers.
