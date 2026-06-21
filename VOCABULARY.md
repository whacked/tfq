# tfq vocabulary

Flat verbs over a collection (a directory of frontmatter'd text files). All
output is JSON.

| Verb | Args | Flags | Output |
|------|------|-------|--------|
| `inspect` | `<file>` | | FileVitals (frontmatter, headings, links, markers) |
| `search` | `<query> <dir>` | `--type T`, `--tag G` | ripgrep hits, frontmatter-filtered |
| `links` | `<ref> <dir>` | | outgoing edges from the record |
| `backlinks` | `<ref> <dir>` | | records that reference `<ref>` |
| `graph` | `<dir>` | | all resolved edges |
| `next` | `<dir>` | | tasks whose dependencies are satisfied |
| `validate` | `<dir>` | `--strict` | validation Report vs `.tfq.cue` |
| `help` | | | usage |

`<ref>` resolves by any key: path, basename, or frontmatter `id`/`slug`/`title`.
Flags may appear anywhere relative to positionals (`--name value`, `--name=value`,
or boolean `--strict`).

Exit codes: `0` success, `1` runtime error or `validate` not OK, `2` usage error.

## Reserved (write ops — not yet implemented)

`read`, `new`, `list`, `set` — deferred to a later pass, along with folding
`tfq` into the agent-resources skills (replacing `ov`/`taskmd`/`cue`).
