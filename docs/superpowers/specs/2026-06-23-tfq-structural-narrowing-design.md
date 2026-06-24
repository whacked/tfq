# tfq — structural narrowing + type discoverability

> Status: approved design (via Q&A), pre-implementation.
> Builds on the flag model (`2026-06-22-tfq-flag-interaction-design.md`).
> Motivation: `--type` was mistaken for a structural-element selector
> (heading/tag/…). It isn't — it filters the frontmatter `type:` field. That
> confusion exposed a real gap: tfq lets you keyword-search (line granularity)
> and query records (record granularity), but offers nothing in between — no way
> to **narrow a noisy search to the structure a match lands in**.

## 1. The querying flow this serves

tfq's unit is a **record = one file**. Search (ripgrep) is the only line-level
op; everything else is record-level. The intended funnel:

1. keyword discover (`tfq X`) → many anywhere-matches
2. **see the shape** of matches (heading? tag? link? prose?)
3. **narrow to structure**
4. reduce to a file set (`-l`/`-c`)
5. record-level graph/status queries (`--links`, `--next`, `--status`, `--show`)

Steps 2–3 are missing today. This pass adds them, plus closes the `--type`
discoverability/consistency gaps.

**Out of scope (decided):** sub-file task tracking (per-`- [ ]`-checkbox
completion). tfq stays one-task-per-file; status is a single frontmatter field.
No new bullet/checkbox extractor this pass.

## 2. Scope

### 2.1 `--in KIND` — structural narrowing of search

`--in heading|tag|link` (repeatable; union). Narrows search hits to those that
land in the named structure. No `--in` = today's anywhere-match.

```bash
tfq battery --in heading        # only hits inside a heading
tfq battery --in tag            # only where "battery" is a tag
tfq battery --in link           # only where it's a link target/label
tfq battery --in heading --in tag
```

Kinds are exactly what tfq already extracts: `heading`, `tag`
(hashtag / org-tag markers **and** frontmatter `tags:`), `link` (any link kind).
`bullet`/`checkbox`/`todo` are **not** offered (not parsed).

### 2.2 Hit labeling — "see the shape"

Each search hit is classified by structure and carries its kinds. Human output
appends a dim `[heading]` / `[tag]` / `[link]` (or `[heading,tag]`) after the
line; prose-only hits are unlabeled. `--json` hits gain an optional
`"kinds": [...]` array. This makes step 2 work *before* you narrow.

### 2.3 `--types` — type discovery

New mode mirroring `--tags`: a count index of the distinct frontmatter `type:`
values in the collection.

```bash
tfq --types
# note  41
# task  12
# log    3
```

### 2.4 `--new` / `--task` write a `type:` field

Templates currently encode note-vs-task only in the *path*, leaving frontmatter
with no `type:` — so `--type` filtering finds nothing on tfq-created records.
Fix: the note template writes `type: note`, the task template `type: task`. An
explicit `--type VALUE` always wins (so `--new x --type log` → `type: log`, note
layout; only `--type task` selects the task layout).

### 2.5 Help / docs clarity

`--type T` is documented as "match the frontmatter `type:` field", distinct from
`--in` (structural element). Update `usage()` and `VOCABULARY.md`.

## 3. Mechanism

### 3.1 Classification (in `internal/search`)

`search.Search` already inspects matched files (`engine.InspectContent`, cached)
when filters are set. Extend it to **always** inspect matched files and classify
each hit by its line against the file's `FileVitals`:

- **heading** — a `Heading.Line == hit.Line`.
- **tag** — a hashtag/org-tag `Marker` on `hit.Line` whose `Value` matches the
  query, OR (when the hit is on a frontmatter `tags:` line) a frontmatter tag
  matching the query.
- **link** — a `Link` on `hit.Line` whose `Target` or `Label` matches the query.

"Matches the query" uses the query compiled as an RE2 regexp (`(?i)` when
`IgnoreCase`); on compile failure, fall back to case-insensitive substring.
Cost is proportional to match count (matched files only, cached) — not corpus
size.

- `search.Filters` gains `In []string`. After classification, a hit is kept iff
  `In` is empty or `kinds ∩ In ≠ ∅`.
- `search.Hit` gains `Kinds []string` (`json:"kinds,omitempty"`); the
  `hits.schema.json` gate adds an optional `kinds` array enum
  `[heading,tag,link]`.

### 3.2 `--types` (in `internal/query`)

`Types(root) ([]TypeCount, error)`, `TypeCount{Type string; Count int}`, sorted
count-desc then name-asc — exactly parallel to `Tags`. New `types.schema.json`
gate + `schema.ValidateTypes`.

### 3.3 `type:` in scaffolds (in `internal/store`)

`scaffold` adds `type` to each template's base map (`note`/`task`) and to the key
order (first field). `cmd` sets `fields["type"] = inv.Type` when `--type` is
given, so an explicit type overrides the template default; the layout template is
still `task` only when `inv.Type == "task"`, else `note`.

### 3.4 CLI (`cmd/tfq`)

- `parse.go`: `Invocation.In []string`; parse repeatable `--in KIND` validating
  `heading|tag|link`; add `ModeTypes` (`--types`).
- `main.go`: thread `In` into `search.Filters`; dispatch `ModeTypes` →
  `query.Types`; `ModeNew` type handling per §3.3; help text updates.
- `format.go`: `formatHits` appends dim kind labels; add a shared count-index
  formatter used by both `formatTagsIndex` and a new `formatTypesIndex`.

## 4. Output examples

```text
$ tfq battery
notes/2026-06-001-battery.md
12: # Battery supply risk   [heading]
14: tracking the #battery rollout   [tag]
31: see [[battery-spec]] for details   [link]
44: the battery degrades under load

$ tfq battery --in heading -l
notes/2026-06-001-battery.md
```

`--json` (one hit):

```json
{"path":"notes/2026-06-001-battery.md","line":12,"text":"# Battery supply risk","kinds":["heading"]}
```

## 5. Test plan (TDD)

- `search`: `--in heading|tag|link` narrowing; `Kinds` populated correctly; a
  prose-only hit has empty kinds; regex + ignore-case classification.
- `schema`: `hits.schema.json` accepts `kinds`; rejects a bad kind enum;
  `ValidateTypes` gate.
- `query`: `Types` counts/sort.
- `store`: `New` writes `type: task`/`type: note`; explicit `--type` override.
- `cmd` `parse_test`: `--in` repeatable + validation; `--types` mode.
- `cmd` `format_test`: hit kind labels (human); types index.
- `cmd` `main_test`: e2e `--in`, `--types`, `--new --type` round-trip
  (`--type task` then `--type task` filter finds it).

`go vet ./... && go test ./...` green before docs.

## 6. Docs

Docs were consolidated this pass: `HANDOFF.md` and `VOCABULARY.md` were folded
into a single brief `README.md` (usage + agent onboarding) and deleted. The
README documents `--in`, hit kinds, `--types`, the `--type` = frontmatter-field
clarity, and that `--new` writes `type:`. Deep design history stays in
`docs/superpowers/specs/` + `plans/`.

## 7. Invariants preserved

- One record per file; no sub-file task model.
- RE2-only, no index, liberal extraction, body+key-order-preserving writes.
- No new third-party dependency. `--json` shapes only *extended* (additive
  `kinds`), keeping schema gates valid.
