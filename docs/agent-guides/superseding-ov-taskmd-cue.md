# Using tfq to supersede ov, taskmd, and cue in agent-resources

For the next agent: this is how to replace the `ov`, `taskmd`, and `cue` binary
dependencies in [agent-resources](https://github.com/whacked/agent-resources)
with a single `tfq` binary. Read `tfq --examples` first for the mental model.

## What tfq covers, and what it does not

| External tool | Role in agent-resources | tfq replacement | Notes |
|---|---|---|---|
| `cue` (required) | `cue vet` of note/report frontmatter against a CUE schema | `tfq --validate FILE --schema TPL` | tfq **bundles** the cuelang library — no `cue` binary. Reads the schema from a `.cue` file *or* a markdown ```cue block. |
| `taskmd` (required) | task records + `add/list/next/set/graph` | `tfq --new --type task`, `--list`, `--next`, `--set`, `--graph` | Same model: one `.md` per record, `dependencies` blocks `--next`. |
| `ov` (optional) | `read/search/list/links/backlinks/tags` over a vault | `tfq --show/<search>/--list/--links/--backlinks/--tags` | tfq is **index-free** — drop `ov index build`. |
| `ck` (optional) | semantic / embedding search | **not covered** — keep `ck` | tfq is RE2/ripgrep; it cannot do embeddings. |

Still needed: `rg` (tfq shells to it; skills may also call it directly, e.g. for
TODO/FIXME scans — that is ripgrep's job, not tfq's), `jq` (for `tfq --json`
post-processing). `cpd` is unrelated and out of scope.

**Deferred (not yet in tfq):** `ov append --section` (structural insert into an
existing file). The notes loop creates + supersedes rather than appending, so
this is not on the critical path. Add it to tfq if a real need appears.

## Command mapping (cheat sheet)

```
ov read "X"                 → tfq --root V --show X            (--raw / --frontmatter)
ov search "kw"              → tfq --root V kw                  (--in heading|tag|link to narrow)
ov list --tag "#t"          → tfq --root V --list --tag t
ov links "X" / backlinks    → tfq --root V --links X / --backlinks X
ov tags                     → tfq --root V --tags
ov index build              → (nothing — tfq has no index)

taskmd add "T" --priority p → tfq --root V --task --title "T" --priority p   [--depends-on a,b --parent x --effort e]
taskmd list --status s      → tfq --root V --list --status s
taskmd next                 → tfq --root V --next
taskmd set ID --done        → tfq --root V --done ID
taskmd set ID --status s    → tfq --root V --set ID --status s
taskmd graph                → tfq --root V --graph

cue vet schema yaml         → tfq --validate FILE --schema TPL
```

`-V` above is the collection root; set it with `--root`, `$TFQ_ROOT`, or a
`.tfq.cue` / `.tfq.yaml` / `.tfq` marker in an ancestor directory.

## Concrete script rewrites

### `scripts/validate-frontmatter.sh` — drop `cue`

Keep the filename and H1-heading-order checks (tfq does **not** do those, by
design). Replace only the `cue vet` call. tfq extracts the ```cue block from the
template itself, so the `extract_cue_schema` awk is no longer needed for it:

```bash
# was:
#   cue vet cue: <(echo "$schema") yaml: <(echo "$yaml")
tfq --validate "$DOCUMENT" --schema "$TEMPLATE"   # exit 0 = valid, 1 = violations
```

`validate-note.sh`'s `command -v cue` branch becomes `command -v tfq`; the
manual required-field fallback can stay as the no-binary degrade path.

### `skills/notes/scripts/new-task.sh` — drop `taskmd`

tfq's default layout already shards into `YYYY/MM/` and assigns padded sequential
ids, so the `taskmd add | jq .file_path | mv` dance collapses to one call. tfq's
`--root` removes taskmd's CWD-config footgun (no `cd` needed):

```bash
tfq --root "$REPO_ROOT/agents/tasks" --json \
    --task --title "$TITLE" --priority "$PRIORITY" ${DEPS:+--depends-on "$DEPS"}
# --title slugifies the path and stores the verbatim title; prints
# {"path":"YYYY/MM/NNN-slug.md","action":"created"}
```

### `skills/doctor/scripts/check.sh` — one binary, no index

Replace the `for bin in ov taskmd ck` loop: require `tfq`, keep `ck` optional,
drop `ov`/`taskmd`. Remove the `ov index status` doc-count checks entirely
(index-free). Frontmatter validation: `tfq --validate "$AGENTS_DIR/notes"`.

### SKILL.md files

`taskmd` and `ov` SKILLs can be folded into the `notes` SKILL (or rewritten to
the tfq verbs above). The `ck` SKILL stays as-is.

## Setup the target collection needs

- **Root marker.** Drop a `.tfq.cue` (or `.tfq`) at the collection root, or pass
  `--root`/`$TFQ_ROOT`. Agent tasks and notes are separate collections
  (`agents/tasks`, `agents/notes`) — point `--root` at the right one per call.
- **Dependency gating works out of the box** — `dependencies` is tfq's default
  blocking edge; no schema required for `--next`. A `.tfq.cue` is only needed to
  *validate* frontmatter or to declare *custom* edge fields (via `@edge`).
- **Schema templates.** Point `--schema` at the existing
  `*.cue.template.md` files; tfq reads their ```cue block directly. The H1
  templates in those files are still consumed by `validate-frontmatter.sh`'s own
  heading check, not by tfq.

## Fidelity notes (verified)

- **Dates.** YAML parses unquoted `date: 2026-06-30` to a timestamp; tfq
  normalizes it back to `YYYY-MM-DD` before CUE validation, so a
  `date: string & =~"..."` schema passes exactly as `cue vet` does.
- **Multi-dependency.** `--depends-on a,b` is written as a real YAML list, so
  each ref resolves as a distinct blocking edge (not a non-resolving `"a,b"`).
- **Obsidian.** `ov index build` also fed *Obsidian's own* backlink panel. tfq
  computes backlinks live for the agent, but does **not** update an Obsidian
  index. If humans browse the vault in Obsidian, that index is now their
  concern (Obsidian maintains it itself) — confirm this is acceptable before
  removing `ov` from a human-facing vault.

## Verify

```bash
tfq --examples                                          # mental model + worked examples
tfq --root agents/tasks --task --title "smoke" --priority low
tfq --root agents/tasks --next                          # the new task is ready
tfq --validate some-note.md --schema skills/notes/schemas/notes.cue.template.md
```
