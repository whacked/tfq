# tfq — flag-based interaction model (verb → flag switchover)

> Status: approved design, pre-implementation.
> Supersedes the CLI grammar in `VOCABULARY.md` and the "flat verbs" decision in
> `HANDOFF.md` §3. Origin: `VOCABULARY-NEW.md` (a UX critique of the shipped
> flat-verb CLI), reconciled with the engine that actually exists today.

## 1. Goal

Replace the flat-verb CLI (`tfq search <query> <dir>`, `tfq read <ref> <dir>`,
…) with a grep-like, flag-based interaction model:

```bash
tfq [OPTIONS] [SELECTOR...]
```

- No verb required; **default mode is search**.
- The non-flag tokens are joined into a single **selector string** (a search
  query, or — depending on mode — a record ref / slug / tag query).
- **No positional `<dir>`**; the collection root is resolved from
  `--root` / `TFQ_ROOT` / an ancestor marker / the cwd.
- **Human-readable output by default** (ripgrep-style); `--json` is the
  universal machine interface.

This is a **clean replacement**: the old verbs are removed entirely. tfq is
pre-release (the agent-resources fold has not happened), so there is no external
caller to keep compatible.

## 2. Scope

### In scope (this pass — "pragmatic core")

**Modes** (exactly one primary mode; default = search):

| Mode flag | Replaces | Selector means | Notes |
|---|---|---|---|
| *(none)* / `--search` | `search` + `list` | search query | empty selector → list behavior |
| `--list` | `list` | optional filter query | always record-summary output |
| `--show` | `read` | record ref | `--raw`, `--frontmatter` |
| `--links` | `links` + `backlinks` | record ref | both directions by default |
| `--tags` | *(new)* | optional tag query | tag index / tag search |
| `--next` | `next` | optional filter | blocking-aware ready tasks |
| `--new` | `new` | slug | `--type`, `--tag`, `--status`, `--field` |
| `--set` | `set` | record ref | mutation flags; ambiguous ref = error |
| `--done` | — | record ref | alias: `--set … --status done` |
| `--validate` | `validate` | — | `--strict` |
| `--inspect` | `inspect` | file path | FileVitals JSON |
| `--graph` | `graph` | — | all edges |
| `--version` | `version` | — | |
| `--help` | `help` | — | |

**Mode aliases:** `--task SLUG` = `--new SLUG --type task`;
`--backlinks REF` = `--links REF --inbound`;
`--outlinks` / `--forward-links REF` = `--links REF --outbound`.

**Filters** (apply to search/list/next): `--type T`, `--tag T` (repeatable,
AND), `--status S`, `--limit N`.

**Search flags:** `-i, --ignore-case`; `-l, --files-with-matches`;
`-c, --count`; `--heading` / `--no-heading`.

**Show flags:** `--raw` (body only), `--frontmatter` (frontmatter only).

**Links flags:** `--inbound`, `--outbound`, `-l` (linked filenames only).

**Universal:** `--json`; `--root DIR`; `-e, --query PATTERN` (explicit selector,
also for selectors beginning with `-`); `--` (stop flag parsing).

### Explicitly deferred (documented as "not yet" in VOCABULARY.md)

Deep ripgrep flags: `-s/--case-sensitive`, `-S/--smart-case`,
`-F/--fixed-strings`, `-w/--word-regexp`, context `-A/-B/-C`, `-o/--only-matching`,
`-v/--invert-match`, `-g/--glob`, `--hidden`, `--no-ignore`, `--sort`,
`-n/-N` line-number toggles.
Structured predicates: `--where k=v`, `--has field`, `--missing field`,
`--before/--after DATE`.
Other: `--depth N` graph expansion, JSON-Lines search format with embedded
record metadata, `--field` list values, `--edit`/`--stdin`/`--body` on `--new`,
`--add-tag`/`--remove-tag`/`--unset`/`--append` beyond what `set` does today.

These are deferred, not rejected; the parser should reject unknown flags with a
usage error rather than silently ignoring them.

## 3. Root resolution (`internal/rootdir`)

New pure, testable package. `Resolve(explicit, env, startDir string) (string, error)`:

1. `explicit` (`--root`) if non-empty.
2. `env` (`TFQ_ROOT`) if non-empty.
3. Nearest ancestor of `startDir` containing `.tfq.cue`, `.tfq.yaml`, or a
   `.tfq/` directory.
4. `startDir` (cwd).

`cmd/tfq` wires `os.Getenv("TFQ_ROOT")` and `os.Getwd()`; tests pass values
directly. `--inspect` does **not** use root resolution — its selector is a file
path used as-is (today's behavior).

## 4. CLI structure (`cmd/tfq`)

Split the current `main.go`/`args.go` into four focused files:

- **`args.go`** — `parse(raw []string) (Invocation, error)`. `Invocation` holds:
  `Mode` (enum), `Selector string`, `Root string`, `JSON bool`, filter fields
  (`Type, Status string; Tags []string; Limit int`), search fields
  (`IgnoreCase, FilesOnly, Count bool; Heading *bool`), show/links/write fields,
  and `Fields map[string]string`. The parser:
  - recognizes short (`-i`, `-l`, `-c`, `-e`) and long flags,
  - maps mode aliases to (mode + implied flags),
  - errors on **two primary modes** with a concrete correction,
  - collects non-flag tokens → selector (joined with spaces), honoring `--`
    and `-e/--query`,
  - errors on unknown flags and on a value-flag missing its value.
- **`main.go`** — `run(args) (string, int)` builds the `Invocation`, resolves
  root, dispatches by mode to `internal/*`, formats, returns `(stdout, exit)`.
- **`format.go`** — human formatters per mode + a `mustJSON` helper. `--json`
  short-circuits to `mustJSON(engineResult)`.
- **`root.go`** — wires `rootdir.Resolve` to the OS.

`run` keeps the `(string, int)` signature for testability; `main` wires it to
`os.Stdout`/`os.Stderr`/`os.Exit`. Errors go to stderr (`tfq: …`), exit 1;
usage errors exit 2; success exit 0; `--validate` not-OK exits 1.

## 5. Engine changes (each TDD'd; output JSON shapes unchanged)

- **`internal/search`**: `Filters` becomes
  `{Type, Status string; Tags []string; IgnoreCase bool}`. `Search` passes `-i`
  to `rg` when `IgnoreCase`; `passesFilters` adds a `status` check and multi-tag
  **AND**. `Hit` is unchanged, so `hits.schema.json` still holds.
- **`internal/query`**: `ListFilters.Tag string` → `Tags []string` (AND). Add
  `Tags(root string) ([]TagCount, error)` where
  `TagCount = {Tag string; Count int}`, sorted by count desc then name. New
  `tags.schema.json` gate for it. `ListItem`/`Record` unchanged.
- **`internal/graph`**: add `Candidates(ref string) []string` — every distinct
  record path the ref could resolve to (path / basename / seq-stripped basename
  / `id`/`slug`/`title`). `Resolve` keeps first-writer-wins.
- **`internal/store`**: `Set` calls `Candidates`; if it returns >1 distinct
  path, return an `ambiguous reference %q (matches: …)` error (exit 1). `New`
  is unaffected (slug, not ref).

`-l` (files-with-matches), `-c` (count per file), and `--limit` are output
projections computed in `cmd/tfq` from the hit list — no engine change.

## 6. Output formats

### Default (human)

- **search hits** (ripgrep TTY style): per-file heading line
  `path  <type> <status> #tag…` then `N: line text` lines, blank line between
  files. `--no-heading` → `path:line:text`. `-l` → one path per line. `-c` →
  `path:count`.
- **list / next**: one block per record — `path  <type> <status> #tag…` and an
  indented `title:` line.
- **tags** (no selector): `tag<pad>count`, sorted. (with selector): grouped
  `# tag  count` headers with member records.
- **links**: record header, then `# outbound links` / `# inbound links`
  sections (`==>` / `<==`). `--inbound`/`--outbound` restrict; `-l` → linked
  paths only.
- **show**: path + frontmatter header + body. `--raw` = body only;
  `--frontmatter` = frontmatter only.
- **inspect / graph / validate / write**: human is a thin summary; the
  authoritative form is `--json`.

### `--json` (universal)

Emits the existing per-mode engine types via `mustJSON` (indented JSON):
`FileVitals`, `[]Hit`, `[]ListItem`, `[]TagCount`, links payload
(`{outbound, inbound}`), `[]Edge`, ready-set, `Record`, `Report`, `WriteResult`.
This keeps every `internal/schema/*_test.go` gate valid. JSON-Lines for search
is deferred (§2).

## 7. Help screen

Replace `usage()` with the teaching-oriented screen from `VOCABULARY-NEW.md`
§"Recommended help screen", trimmed to the flags actually shipped (no deferred
flags advertised).

## 8. Test plan (TDD)

- `internal/rootdir`: table test for the 4-step resolution + ancestor walk.
- `internal/search`: ignore-case hit; multi-tag AND; status filter.
- `internal/query`: multi-tag AND list; `Tags` index counts/sort; `tags` schema
  gate.
- `internal/graph`: `Candidates` returns all matches for an ambiguous ref.
- `internal/store`: `Set` errors on ambiguous ref; unchanged on unique ref.
- `cmd/tfq` `args_test.go`: parser table — default search, each mode, aliases,
  selector joining, `--`, `-e`, two-mode error, unknown-flag error, missing
  value error.
- `cmd/tfq` `format_test.go`: heading vs `--no-heading`; `-l`; `-c`; links
  both-directions; tags index; list block.
- `cmd/tfq` `main_test.go`: rewritten end-to-end per mode using `--root TMP`,
  covering the HANDOFF "real workflow" (new → set dep → next gates → done →
  list reflects state) in the new grammar, for both human and `--json`.

`go vet ./... && go test ./...` green before docs.

## 9. Docs (after code is green)

- Rewrite **`VOCABULARY.md`** to the flag model (modes, filters, search flags,
  output, root policy, deferred list, exit codes).
- Update **`HANDOFF.md`**: §1 verb surface → flag surface; §3 "CLI grammar"
  row; §4 `cmd/tfq` package description; §10 remove "human output deferred";
  §13 quickref examples.
- Delete **`VOCABULARY-NEW.md`**.
- This spec is the durable design record.

## 10. Non-goals / invariants preserved

- Engine packages keep their responsibilities and output JSON shapes.
- RE2-only, no search index, liberal extraction, body+key-order-preserving
  writes, the `layout` config seam — all unchanged.
- No new third-party dependency; parser stays hand-rolled.
