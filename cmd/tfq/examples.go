package main

import "strings"

// helpExample is one agent-facing example: a command (args after "tfq") and the
// exact output it produces against the sample collection built in
// examples_test.go. The extended help renders these, and a test runs every one
// and asserts the output still matches Want — so the help cannot drift from real
// behavior without breaking the build.
type helpExample struct {
	Desc string
	Args []string
	Want string
}

// examples are deterministic read/query commands (no dates or absolute paths in
// their output) so the rendered help stays byte-stable and test-verifiable.
var examples = []helpExample{
	{
		Desc: "discover: keyword search labels each hit by the structure it landed in",
		Args: []string{"battery"},
		Want: "battery.md\n8: the battery degrades under load\n\ncells.md\n6: notes on [[battery]] internals [link]\n\ntask-audit.md\n7: # Audit battery vendors [heading]",
	},
	{
		Desc: "narrow: keep only matches inside a heading, files only",
		Args: []string{"battery", "--in", "heading", "-l"},
		Want: "task-audit.md",
	},
	{
		Desc: "case-insensitive, files with matches",
		Args: []string{"-i", "-l", "battery"},
		Want: "battery.md\ncells.md\ntask-audit.md",
	},
	{
		Desc: "tag index across the collection",
		Args: []string{"--tags"},
		Want: "# tags\n  power         2\n  supply-chain  1",
	},
	{
		Desc: "frontmatter type: index",
		Args: []string{"--types"},
		Want: "# types\n  note  2\n  task  1",
	},
	{
		Desc: "ready tasks (dependencies satisfied)",
		Args: []string{"--next"},
		Want: "task-audit.md  task pending\n  title: Audit battery vendors",
	},
	{
		Desc: "who links to this record",
		Args: []string{"--backlinks", "battery"},
		Want: "battery\n\n# inbound links\n  <== cells.md",
	},
	{
		Desc: "one record's frontmatter",
		Args: []string{"--show", "task-audit", "--frontmatter"},
		Want: "id: 001\npriority: high\nstatus: pending\ntype: task",
	},
}

// extendedHelp is the agent-facing guide: the mental model, the querying funnel,
// the supersession scope, and every yoked example with its real output.
func extendedHelp() string {
	var b strings.Builder
	b.WriteString(`tfq — extended help (for agents)

tfq treats a directory of frontmatter'd text files as records forming a typed
graph. One record = one file. Search (ripgrep) is the only line-level op;
everything else is record-level. No index, no services.

The querying funnel:
  1. discover   tfq KEYWORD             ripgrep-style anywhere-matches
  2. see shape  [heading] [tag] [link]  each hit is labeled by where it landed
  3. narrow     --in heading|tag|link   keep only matches in that structure
  4. reduce     -l (files)  -c (counts)
  5. traverse   --links --backlinks --next --graph    record-level graph queries

Supersession scope:
  ov     read/search/list/links/backlinks/tags — index-free (no 'index build')
  taskmd records + dependencies + --next/--graph (same one-file-per-record model)
  cue    bundled — tfq --validate [FILE] --schema PATH (no cue binary needed)
  ck     NOT covered — semantic/embedding search; use ck or rg for that

Reference selectors resolve by path, basename, seq-stripped basename, or
frontmatter id/slug/title. Exit codes: 0 ok · 1 runtime/validate-fail · 2 usage.

Examples (run against a small sample collection):
`)
	for _, ex := range examples {
		b.WriteString("\n# " + ex.Desc + "\n")
		b.WriteString("$ tfq " + strings.Join(ex.Args, " ") + "\n")
		b.WriteString(ex.Want + "\n")
	}
	b.WriteString(`
Writing & validating (output depends on date/path, so not shown):
  tfq --new SLUG --type note --tag x                  create a note (SLUG is [a-z0-9-]+)
  tfq --task --title "Audit vendors" --priority high  create a task; title auto-slugs the path
  tfq --set REF --status in-progress                  update frontmatter
  tfq --set REF --depends-on 001,002                  set blocking dependencies
  tfq --done REF                                      mark a task done
  tfq --validate FILE --schema notes.cue.template.md  validate one file (cue vet)
  tfq --validate [--strict]                           validate the whole collection

See docs/agent-guides/ for the ov/taskmd/cue → tfq migration guide.`)
	return b.String()
}
