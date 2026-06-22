# tfq vocabulary

Flat verbs over a collection (a directory of frontmatter'd text files). All
query output is JSON.

| Verb | Args | Flags | Output |
|------|------|-------|--------|
| `inspect` | `<file>` | | FileVitals (frontmatter, headings, links, markers) |
| `read` | `<ref> <dir>` | `--raw` | record (frontmatter + body), or `--raw` body text only |
| `search` | `<query> <dir>` | `--type T`, `--tag G` | ripgrep hits, frontmatter-filtered |
| `list` | `<dir>` | `--status S`, `--tag T`, `--type T` | record summaries, filtered |
| `links` | `<ref> <dir>` | | outgoing edges from the record |
| `backlinks` | `<ref> <dir>` | | records that reference `<ref>` |
| `graph` | `<dir>` | | all resolved edges |
| `next` | `<dir>` | | tasks whose dependencies are satisfied |
| `new` | `<slug> <dir>` | `--template note\|task`, `--field k=v` (repeatable) | WriteResult (created) |
| `set` | `<ref> <dir>` | `--status S`, `--add-tag T` (repeatable), `--field k=v` (repeatable) | WriteResult (updated) |
| `validate` | `<dir>` | `--strict` | validation Report vs `.tfq.cue` |
| `version` | | | build version `yyyymmdd.n.hash` (injected via Makefile) |
| `help` | | | usage |

`<ref>` resolves by any key: path, basename, basename with a leading `NNN-`
sequence stripped (so a task `001-do-thing.md` resolves as `do-thing`), or
frontmatter `id`/`slug`/`title`. Flags may appear anywhere relative to
positionals (`--name value`, `--name=value`, or boolean `--strict`/`--raw`).

Exit codes: `0` success, `1` runtime error or `validate` not OK, `2` usage error.

## Path policy

`new` shards and names files via `internal/layout` — a single config seam.
Defaults match the agent-resources conventions:

- note → `{yyyy}/{mm}/{yyyy}-{mm}-{dd}.{nnn}-{slug}.md` (daily sequence)
- task → `{yyyy}/{mm}/{nnn}-{slug}.md` (global sequence)

The `Config` struct is built to accept user-supplied rules in a future pass.

## Still deferred

Folding `tfq` into the agent-resources skills (replacing `ov`/`taskmd`/`cue`
in the skill docs, `new-task.sh`, `validate-note.sh`, and `doctor`) — a
separate pass that edits the live external repo and warrants a report.
