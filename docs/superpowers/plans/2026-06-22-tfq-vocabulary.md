# tfq Vocabulary Implementation Plan (Phase 4a — vocabulary only)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Finalize `tfq`'s flat-verb CLI vocabulary over the existing read-only operations: a robust flags-anywhere parser, expose already-built-but-unreachable capability (`search --type/--tag`, a `links` verb), and a documented `help`. No write ops, no agent-resources changes.

**Architecture:** Replace the ad-hoc positional-only dispatch in `cmd/tfq/main.go` with a small `partition` helper that splits args into positionals + flags (supporting `--flag val`, `--flag=val`, and boolean flags in any position), then per-verb handlers consume positionals + typed flags. Output stays JSON (agent-first); human formatting is out of scope.

**Tech Stack:** Go 1.25. No new dependencies. Builds on existing `engine`/`graph`/`search`/`validate`/`scan`/`cueschema`.

## Global Constraints

- **Flat verbs only.** Final surface: `inspect`, `search`, `links`, `backlinks`, `graph`, `next`, `validate`, `help`.
- **Flags anywhere** relative to positionals: `tfq search foo dir --type note` and `tfq search --type note foo dir` both work.
- Output is JSON; exit codes: 0 success, 1 runtime error / validate-not-OK, 2 usage error.
- No write operations (`read`/`new`/`list`/`set` are reserved, documented as not-yet-implemented).
- No changes outside this repo.

---

### Task 1: arg partition helper

**Files:**
- Create: `cmd/tfq/args.go`
- Test: `cmd/tfq/args_test.go`

**Interfaces:**
- Produces: `func partition(raw []string, bools map[string]bool) (pos []string, flags map[string]string, err error)`.
  - `--name=value` → `flags["name"]="value"`.
  - `--name` where `bools["name"]` → `flags["name"]="true"`.
  - `--name value` (non-bool) → `flags["name"]="value"`, consuming the next token.
  - non-`--` tokens → appended to `pos` (order preserved).
  - a non-bool flag with no following value → error.

- [ ] **Step 1: Write the failing test**

```go
// cmd/tfq/args_test.go
package main

import (
	"reflect"
	"testing"
)

func TestPartition(t *testing.T) {
	bools := map[string]bool{"strict": true}

	pos, flags, err := partition([]string{"foo", "dir", "--type", "note", "--strict"}, bools)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pos, []string{"foo", "dir"}) {
		t.Errorf("pos = %#v", pos)
	}
	if flags["type"] != "note" || flags["strict"] != "true" {
		t.Errorf("flags = %#v", flags)
	}

	// flags before positionals, and --k=v form
	pos, flags, err = partition([]string{"--tag=urgent", "ref", "dir"}, bools)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pos, []string{"ref", "dir"}) || flags["tag"] != "urgent" {
		t.Errorf("pos=%#v flags=%#v", pos, flags)
	}

	// non-bool flag missing a value -> error
	if _, _, err := partition([]string{"--type"}, bools); err == nil {
		t.Error("expected error for --type with no value")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/tfq/... -run TestPartition`
Expected: FAIL (`partition` undefined).

- [ ] **Step 3: Write the implementation**

```go
// cmd/tfq/args.go
package main

import (
	"fmt"
	"strings"
)

// partition splits raw CLI args into positionals and flags. bools lists flags
// that take no value. Supports --name=value, --name value, and --bool in any
// position relative to positionals.
func partition(raw []string, bools map[string]bool) ([]string, map[string]string, error) {
	pos := []string{}
	flags := map[string]string{}
	for i := 0; i < len(raw); i++ {
		a := raw[i]
		if !strings.HasPrefix(a, "--") {
			pos = append(pos, a)
			continue
		}
		name := a[2:]
		if eq := strings.IndexByte(name, '='); eq >= 0 {
			flags[name[:eq]] = name[eq+1:]
			continue
		}
		if bools[name] {
			flags[name] = "true"
			continue
		}
		if i+1 >= len(raw) {
			return nil, nil, fmt.Errorf("flag --%s needs a value", name)
		}
		flags[name] = raw[i+1]
		i++
	}
	return pos, flags, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/tfq/... -run TestPartition`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/tfq/args.go cmd/tfq/args_test.go
git commit -m "feat(cmd): flags-anywhere arg partition helper"
```

---

### Task 2: flat-verb dispatch + links/help + search filters

**Files:**
- Modify: `cmd/tfq/main.go`
- Test: `cmd/tfq/main_test.go` (add cases)
- Create: `VOCABULARY.md`

**Interfaces:**
- Consumes: `partition`, existing engine/graph/search/validate.
- Produces final verbs: `inspect <file>`, `search <query> <dir> [--type T] [--tag G]`, `links <ref> <dir>`, `backlinks <ref> <dir>`, `graph <dir>`, `next <dir>`, `validate <dir> [--strict]`, `help`.

- [ ] **Step 1: Write the failing tests (add to `cmd/tfq/main_test.go`)**

```go
func TestRunSearchFilters(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "a.md", "---\ntype: note\n---\nhello world\n")
	mustWrite(t, dir, "b.md", "---\ntype: log\n---\nhello again\n")

	out, code := run([]string{"search", "hello", dir, "--type", "log"})
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	if !contains(out, "b.md") || contains(out, "a.md") {
		t.Errorf("--type filter not applied: %s", out)
	}
}

func TestRunLinks(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "a.md", "---\nslug: a\n---\nsee [[b]]\n")
	mustWrite(t, dir, "b.md", "---\nslug: b\n---\n# b\n")

	out, code := run([]string{"links", "a", dir})
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	if !contains(out, "\"to\": \"b.md\"") {
		t.Errorf("links should show forward edge to b.md: %s", out)
	}
}

func TestRunHelp(t *testing.T) {
	out, code := run([]string{"help"})
	if code != 0 {
		t.Errorf("help should exit 0, got %d", code)
	}
	for _, verb := range []string{"inspect", "search", "links", "backlinks", "graph", "next", "validate"} {
		if !contains(out, verb) {
			t.Errorf("help missing verb %q", verb)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./cmd/tfq/... -run 'TestRunSearchFilters|TestRunLinks|TestRunHelp'`
Expected: FAIL (`links`/`help` unknown → exit 2; `--type` ignored).

- [ ] **Step 3: Rewrite `run` in `cmd/tfq/main.go`**

```go
func run(args []string) (string, int) {
	if len(args) < 1 {
		return usage(), 2
	}
	verb, rest := args[0], args[1:]
	switch verb {
	case "help":
		return usage(), 0
	case "inspect":
		pos, _, err := partition(rest, nil)
		if err != nil || len(pos) != 1 {
			return usage(), 2
		}
		fv, ierr := engine.Inspect(pos[0])
		if ierr != nil {
			return errln(ierr), 1
		}
		return mustJSON(fv), 0
	case "search":
		pos, flags, err := partition(rest, nil)
		if err != nil || len(pos) != 2 {
			return usage(), 2
		}
		hits, _, serr := search.Search(pos[1], pos[0], search.Filters{Type: flags["type"], Tag: flags["tag"]})
		if serr != nil {
			return errln(serr), 1
		}
		return mustJSON(hits), 0
	case "links":
		pos, _, err := partition(rest, nil)
		if err != nil || len(pos) != 2 {
			return usage(), 2
		}
		g, gerr := buildGraph(pos[1])
		if gerr != nil {
			return errln(gerr), 1
		}
		return mustJSON(g.Forward(pos[0])), 0
	case "backlinks":
		pos, _, err := partition(rest, nil)
		if err != nil || len(pos) != 2 {
			return usage(), 2
		}
		g, gerr := buildGraph(pos[1])
		if gerr != nil {
			return errln(gerr), 1
		}
		return mustJSON(g.Backlinks(pos[0])), 0
	case "graph":
		pos, _, err := partition(rest, nil)
		if err != nil || len(pos) != 1 {
			return usage(), 2
		}
		g, gerr := buildGraph(pos[0])
		if gerr != nil {
			return errln(gerr), 1
		}
		return mustJSON(g.Edges()), 0
	case "next":
		pos, _, err := partition(rest, nil)
		if err != nil || len(pos) != 1 {
			return usage(), 2
		}
		g, gerr := buildGraph(pos[0])
		if gerr != nil {
			return errln(gerr), 1
		}
		ready, _ := g.Next(graph.DefaultNextOptions())
		return mustJSON(ready), 0
	case "validate":
		pos, flags, err := partition(rest, map[string]bool{"strict": true})
		if err != nil || len(pos) != 1 {
			return usage(), 2
		}
		rep, verr := validate.Run(pos[0], flags["strict"] == "true")
		if verr != nil {
			return errln(verr), 1
		}
		code := 0
		if !rep.OK {
			code = 1
		}
		return mustJSON(rep), code
	default:
		return usage(), 2
	}
}

func errln(err error) string {
	fmt.Fprintln(os.Stderr, "tfq: "+err.Error())
	return ""
}
```

Replace `usage()` with the documented vocabulary:

```go
func usage() string {
	return strings.Join([]string{
		"tfq — query frontmatter'd text files",
		"",
		"usage: tfq <verb> [args] [flags]",
		"",
		"  inspect <file>                    comprehensive FileVitals JSON for one file",
		"  search <query> <dir> [--type T] [--tag G]   ripgrep search + frontmatter filters",
		"  links <ref> <dir>                 outgoing edges from a record",
		"  backlinks <ref> <dir>             records that reference <ref>",
		"  graph <dir>                       all resolved edges in the collection",
		"  next <dir>                        tasks ready to work on (deps satisfied)",
		"  validate <dir> [--strict]         validate vs .tfq.cue + edge resolution",
		"  help                              this message",
		"",
		"reserved (not yet implemented): read, new, list, set",
	}, "\n")
}
```

Add `"strings"` to the imports in `main.go`.

- [ ] **Step 4: Run tests + build + smoke**

Run: `go test ./cmd/tfq/...`
Expected: PASS.

Run: `go build -o tfq ./cmd/tfq && ./tfq help && ./tfq links note-a internal/scan/testdata/vault`
Expected: usage text; then a forward-edge array (note-a → sub/note-b.org).

- [ ] **Step 5: Write `VOCABULARY.md`**

```markdown
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
Flags may appear anywhere relative to positionals.

Exit codes: `0` success, `1` runtime error or `validate` not OK, `2` usage error.

## Reserved (write ops — not yet implemented)

`read`, `new`, `list`, `set` — deferred to a later pass, along with folding
`tfq` into the agent-resources skills (replacing `ov`/`taskmd`/`cue`).
```

- [ ] **Step 6: Final full-suite run and commit**

Run: `go vet ./... && go test ./...`
Expected: all packages PASS.

```bash
git add cmd/tfq VOCABULARY.md
git commit -m "feat(cmd): finalize flat-verb vocabulary (links, help, search filters)"
```

---

## Self-Review

- **Scope:** vocabulary-only as chosen — flat verbs, flags-anywhere, exposes existing search filters + forward links, documented help/VOCABULARY.md. No write ops, no agent-resources edits. ✓
- **Placeholders:** none — runnable code + expected output in every step.
- **Type consistency:** `partition`, `errln`, `mustJSON`, `buildGraph`, and the existing `engine`/`graph`/`search`/`validate` calls are used consistently. `links` uses the existing `graph.Forward`; `search` filters use the existing `search.Filters`.
