# tfq

`tfq` (text-file query) is a single Go binary that treats a directory of
frontmatter'd text files (Markdown, Org, …) as **records forming a typed graph**,
and exposes read + write + validate over them. No index, no services; search is
ripgrep over plaintext. It replaces the durable parts of `ov` (notes) and
`taskmd` (tasks).

```bash
make build      # -> ./tfq (version injected)
make test       # go test ./...
./tfq --help
```

## Using tfq

Grep-like: the non-flag tokens are a **selector**; flags do the rest. Default
mode is search. Output is human (colored on a TTY); add `--json` for tooling.

```bash
tfq battery supply              # search record bodies (ripgrep)
tfq battery --in heading        # narrow to matches inside a heading (or tag|link)
tfq -i -l battery               # case-insensitive, files only
tfq --status pending            # list records (empty selector + filter)
tfq --show battery-spec         # one record (--raw / --frontmatter)
tfq --links battery-spec        # outbound + inbound (--inbound/--outbound)
tfq --next                      # tasks whose deps are satisfied
tfq --new idea --type note --tag x   # create; --task = --new --type task
tfq --set idea --status done    # mutate frontmatter (--done is a shortcut)
tfq --tags / --types            # tag and frontmatter-type indexes
tfq --validate [--strict]       # check vs .tfq.cue
```

| Group | Flags |
|---|---|
| Modes | *(search)* `--list --show --links --tags --types --next --new --set --done --validate --inspect --graph --version --help` |
| Filters | `--type T` (frontmatter `type:`) · `--tag T`× · `--status S` · `--limit N` |
| Search | `-i` · `-l` · `-c` · `--heading/--no-heading` · `--in heading\|tag\|link` |
| Root | `--root DIR` → `$TFQ_ROOT` → ancestor `.tfq.cue/.tfq.yaml/.tfq/` → cwd |
| Output | `--json` · `--color auto\|always\|never` / `--no-color` (honors `NO_COLOR`) · `-e/--query` · `--` |

`<ref>` resolves by path, basename, seq-stripped basename (`001-x.md`→`x`), or
frontmatter `id`/`slug`/`title`. Writes hard-error on an ambiguous ref. Exit
codes: `0` ok · `1` runtime / validate-not-ok · `2` usage.

## Working in this repo

**Mental model:** a *collection* is a directory; each file is a *record* =
`{path, frontmatter, headings, links, markers}`. Edges come from body links
(`[[wiki]]`, markdown, org) and configurable frontmatter fields
(`dependencies`, `parent`, …); the *graph* resolves edge targets against a
multi-key index. One record = one file (no sub-file task model).

**Packages** (`internal/…`):

| Package | Role |
|---|---|
| `model` | output contract types (`FileVitals`, `Link`, `Marker`, `Heading`) |
| `extract` | four liberal RE2 extractors (frontmatter, headings, links, markers) — never fail |
| `engine` · `scan` | inspect one file → `FileVitals`; walk a dir → `[]FileVitals` |
| `graph` | multi-key index + edge resolution + `Backlinks/Forward/Next/Candidates` |
| `search` | ripgrep wrapper + frontmatter filters + `--in` structural classification |
| `query` · `store` | read (`List/Read/Tags/Types/Summarize`) · write (`New/Set`, body-preserving) |
| `cueschema` · `validate` | load `.tfq.cue`, `@edge` fields, liberal/strict report |
| `layout` | path/sharding/ID config seam (`DefaultConfig` = agent-resources conventions) |
| `rootdir` | collection-root resolution |
| `cmd/tfq` | `parse.go` (flags→`Invocation`) → `main.go` (dispatch) → `format.go` (human) / `style.go` (color) |

**Invariants:** RE2 only (Go = ripgrep = CUE `=~`); extractors never fatal;
writes preserve body + key order; `--json` shapes are gated against
`internal/schema/*.schema.json` in tests — a mode without a passing schema test
is not done. No third-party CLI framework.

**Method:** TDD, one commit per task. Design specs and TDD plans (the deepest
record of *what* and *why*) live in `docs/superpowers/specs/` and
`docs/superpowers/plans/`.
