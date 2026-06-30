# tfq

`tfq` (text-file query) is a single Go binary that treats a directory of
frontmatter'd text files (Markdown, Org, …) as **records forming a typed graph**,
and exposes read + write + validate over them. No index, no services; search is
ripgrep over plaintext. One binary supersedes `ov` (notes — index-free),
`taskmd` (tasks + dependency graph), and `cue` (frontmatter validation, via the
bundled cuelang library). Semantic search (`ck`) is out of scope.

```bash
make build      # -> ./tfq (version injected)
make test       # go test ./...
./tfq --help        # short flag reference
./tfq --examples    # extended agent guide: mental model + worked examples
```

## Install / run with Nix

A flake is provided; it bundles `ripgrep`, so search works out of the box.

```bash
nix run github:whacked/tfq -- --help        # run without installing
nix run github:whacked/tfq -- battery --in heading
nix profile install github:whacked/tfq      # install `tfq` onto PATH
nix build github:whacked/tfq                # -> ./result/bin/tfq
```

Dev shell (`go`, `nodejs`, `pkg-config`, shortcuts) — both entry points give the
same environment, defined once in `flake.nix`:

```bash
nix develop     # flakes
nix-shell       # non-flake; shell.nix is a flake-compat shim onto the flake
```

Flake builds derive the version from flake metadata
(`yyyymmdd.<shortRev>`); `make build` keeps the fuller git-derived string.

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
tfq --task --title "Audit vendors" --priority high --depends-on 001,002
tfq --set idea --status done    # mutate frontmatter (--done is a shortcut)
tfq --tags / --types            # tag and frontmatter-type indexes
tfq --validate note.md --schema notes.cue.template.md   # one file, cue vet semantics
tfq --validate [--strict]       # whole collection vs discovered .tfq.cue
```

| Group | Flags |
|---|---|
| Modes | *(search)* `--list --show --links --tags --types --next --new --set --done --validate --inspect --graph --version --help --examples` |
| Filters | `--type T` (frontmatter `type:`) · `--tag T`× · `--status S` · `--limit N` |
| Task fields (`--new`/`--set`) | `--title T` (auto-slugs) · `--priority P` · `--effort E` · `--parent REF` · `--depends-on REF[,REF]` · `--field k=v` |
| Search | `-i` · `-l` · `-c` · `--heading/--no-heading` · `--in heading\|tag\|link` |
| Validate | `--strict` · `--schema PATH` (`.cue` or a markdown ```` ```cue ```` block; with a FILE selector validates one file) |
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
`docs/superpowers/plans/`. To wire tfq into agent-resources (replacing
`ov`/`taskmd`/`cue`), see
[`docs/agent-guides/superseding-ov-taskmd-cue.md`](docs/agent-guides/superseding-ov-taskmd-cue.md).
