# tfq vocabulary

A grep-like interface over a collection (a directory of frontmatter'd text
files). No verbs: the first non-flag tokens form a **selector string**, flags
do everything else.

```
tfq [OPTIONS] [SELECTOR...]
```

Default output is **human-readable** (ripgrep-style); `--json` is the universal
machine interface. The default mode is **search**.

## Modes

Exactly one primary mode flag is allowed; omit it for search. The selector means
different things per mode (search query, record ref, slug, or tag query).

| Mode | Selector | Key flags | Replaces (old verb) |
|------|----------|-----------|---------------------|
| *(default)* / `--search` | search query | `-i`, `-l`, `-c`, filters | `search` + `list` |
| `--list` | optional filter | filters | `list` |
| `--show` | record ref | `--raw`, `--frontmatter` | `read` |
| `--links` | record ref | `--inbound`, `--outbound`, `-l` | `links` + `backlinks` |
| `--tags` | optional tag query | — | *(new)* |
| `--next` | optional filter | filters | `next` |
| `--new` | slug | `--type`, `--tag`, `--status`, `--field` | `new` |
| `--set` | record ref | `--status`, `--tag`, `--field` | `set` |
| `--done` | record ref | — | *(alias of `--set … --status done`)* |
| `--validate` | — | `--strict` | `validate` |
| `--inspect` | file path | — | `inspect` |
| `--graph` | — | — | `graph` |
| `--version` | — | — | `version` |
| `--help` | — | — | `help` |

**Mode aliases:** `--task SLUG` = `--new SLUG --type task`;
`--backlinks REF` = `--links REF --inbound`;
`--outlinks` / `--forward-links REF` = `--links REF --outbound`.

The default mode folds the old `search` and `list`: a **non-empty** selector
greps record bodies (ripgrep hits); an **empty** selector with filters lists
record summaries. `--list` forces summary output even with a selector (matched
as a case-insensitive substring on path/title).

## Selector

Non-flag tokens are joined with spaces into the selector. `--` stops flag
parsing; `-e, --query PATTERN` supplies an explicit selector (also for patterns
beginning with `-`).

```bash
tfq battery supply chain          # selector = "battery supply chain"
tfq --tag battery supply -i       # tag=battery, ignore-case, selector="supply"
tfq -e "-foo"                     # selector = "-foo"
tfq -- -foo                       # selector = "-foo"
```

## Filters

Apply to search / list / next (AND semantics; empty matches all):

```
--type T            frontmatter type
--tag T             repeatable; record must carry all given tags
--status S          frontmatter status
--limit N           cap results
```

In `--new`/`--set` the same flags are *writes*, not filters: `--type` selects
the template, `--tag` adds tags, `--status` sets status, `--field k=v`
(repeatable) sets a scalar field.

## Search flags

```
-i, --ignore-case
-l, --files-with-matches    paths only
-c, --count                 path:count
--heading / --no-heading    file heading (default) vs path:line:text
```

Default human output is ripgrep-style: a file-path heading, then `line: text`
hits, a blank line between files. `--no-heading` emits `path:line:text`.

## Collection root

There is **no positional directory**. The root resolves in order:

```
--root DIR
$TFQ_ROOT
nearest ancestor containing .tfq.cue / .tfq.yaml / .tfq/
current directory
```

`--inspect` is the exception: its selector is a literal file path, used as-is.

## Color

Human output is colored by default when stdout is a terminal, and plain when
piped or redirected (ripgrep-like). Paths are magenta, line numbers green, the
matched substring bold red; list/tags/links/validate get tasteful tints.

```
--color auto      colored on a TTY, plain when piped (default)
--color always    force color (overrides NO_COLOR)
--color never     force plain
--no-color        = --color never
```

`NO_COLOR` (any non-empty value) disables color under `--color auto`. `--json`
output is never colored.

## Output

Human by default. `--json` emits the per-mode engine types (indented JSON):
`FileVitals` (inspect), `[]Hit` (search; `[]string` with `-l`, `[{path,count}]`
with `-c`), `[]ListItem` (list/next), `[]TagCount` / `[]TagGroup` (tags),
`{path,outbound,inbound}` (links), `[]Edge` (graph), `Record` (show), `Report`
(validate), `WriteResult` (new/set). Every engine output type is gated against a
JSON Schema in `internal/schema/*_test.go`.

## Resolution & writes

`<ref>` resolves by any key: path, basename, basename with a leading `NNN-`
sequence stripped (so a task `001-do-thing.md` resolves as `do-thing`), or
frontmatter `id`/`slug`/`title`. Reads pick the first match; **writes
(`--set`/`--done`) hard-error on an ambiguous reference** (more than one record
matches) rather than silently picking one.

Exit codes: `0` success, `1` runtime error or `validate` not OK, `2` usage error.

## Path policy

`--new` shards and names files via `internal/layout` — a single config seam.
Defaults match the agent-resources conventions:

- note → `{yyyy}/{mm}/{yyyy}-{mm}-{dd}.{nnn}-{slug}.md` (daily sequence)
- task → `{yyyy}/{mm}/{nnn}-{slug}.md` (global sequence)

The `Config` struct is built to accept user-supplied rules in a future pass.

## Deferred (not yet implemented)

The interaction model is intentionally a pragmatic core. These are documented
as future work, not rejected:

- Deep ripgrep flags: `-s/--case-sensitive`, `-S/--smart-case`,
  `-F/--fixed-strings`, `-w/--word-regexp`, context `-A/-B/-C`,
  `-o/--only-matching`, `-v/--invert-match`, `-g/--glob`, `--hidden`,
  `--no-ignore`, `--sort`, `-n/-N` line-number toggles.
- Structured predicates: `--where k=v`, `--has field`, `--missing field`,
  `--before/--after DATE`.
- `--depth N` graph expansion, JSON-Lines search format with embedded record
  metadata, metadata enrichment in search headings, `--field` list values,
  `--edit`/`--stdin`/`--body` on `--new`, `--remove-tag`/`--unset`/`--append`
  on `--set`.

## Still deferred (strategic)

Folding `tfq` into the agent-resources skills (replacing `ov`/`taskmd`/`cue`
in the skill docs, `new-task.sh`, `validate-note.sh`, and `doctor`) — a
separate pass that edits the live external repo and warrants a report.
