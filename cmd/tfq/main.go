package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"tfq/internal/cueschema"
	"tfq/internal/engine"
	"tfq/internal/graph"
	"tfq/internal/layout"
	"tfq/internal/model"
	"tfq/internal/query"
	"tfq/internal/scan"
	"tfq/internal/search"
	"tfq/internal/store"
	"tfq/internal/validate"
)

// version is the build version (yyyymmdd.<nth-commit-of-day>.<hash>); overridden
// at build time via -ldflags (see Makefile). Defaults to "dev".
var version = "dev"

type linksJSON struct {
	Path     string       `json:"path"`
	Outbound []graph.Edge `json:"outbound,omitempty"`
	Inbound  []string     `json:"inbound,omitempty"`
}

// run returns (stdoutText, exitCode). Pure and color-free for testing; main
// calls runEnv with the real terminal/NO_COLOR state.
func run(args []string) (string, int) {
	return runEnv(args, false, false)
}

func runEnv(args []string, isTTY, noColor bool) (string, int) {
	if len(args) == 0 {
		return usage(), 0
	}
	inv, err := parse(args)
	if err != nil {
		if ue, ok := err.(usageError); ok {
			return "tfq: " + ue.Error() + "\n\n" + usage(), 2
		}
		return errln(err), 1
	}
	pal := palette{on: decideColor(inv.Color, noColor, isTTY)}

	switch inv.Mode {
	case ModeHelp:
		return usage(), 0
	case ModeVersion:
		return version, 0
	}

	// --inspect operates on the selector as a literal file path.
	if inv.Mode == ModeInspect {
		if inv.Selector == "" {
			return needSelector("--inspect")
		}
		fv, ierr := engine.Inspect(inv.Selector)
		if ierr != nil {
			return errln(ierr), 1
		}
		if inv.JSON {
			return mustJSON(fv), 0
		}
		return formatInspect(fv, pal), 0
	}

	root, rerr := resolveRoot(inv.Root)
	if rerr != nil {
		return errln(rerr), 1
	}

	switch inv.Mode {
	case ModeSearch:
		if inv.Selector == "" {
			return dispatchList(root, inv, pal)
		}
		hits, _, serr := search.Search(root, inv.Selector, search.Filters{
			Type: inv.Type, Status: inv.Status, Tags: inv.Tags, In: inv.In, IgnoreCase: inv.IgnoreCase})
		if serr != nil {
			return errln(serr), 1
		}
		if inv.Limit > 0 && len(hits) > inv.Limit {
			hits = hits[:inv.Limit]
		}
		if inv.FilesOnly {
			files := filesOf(hits)
			if inv.JSON {
				return mustJSON(files), 0
			}
			return formatPaths(files, pal), 0
		}
		if inv.Count {
			counts := countsOf(hits)
			if inv.JSON {
				return mustJSON(counts), 0
			}
			return formatCounts(counts, pal), 0
		}
		if inv.JSON {
			return mustJSON(hits), 0
		}
		return formatHits(hits, inv.Heading, matcher(inv, pal), pal), 0

	case ModeList:
		return dispatchList(root, inv, pal)

	case ModeShow:
		if inv.Selector == "" {
			return needSelector("--show")
		}
		rec, qerr := query.Read(root, inv.Selector)
		if qerr != nil {
			return errln(qerr), 1
		}
		if inv.JSON {
			return mustJSON(rec), 0
		}
		if inv.Raw {
			return rec.Body, 0
		}
		if inv.Frontmatter {
			return formatFrontmatterBlock(rec.Frontmatter, pal), 0
		}
		return formatRecord(rec, pal), 0

	case ModeLinks:
		if inv.Selector == "" {
			return needSelector("--links")
		}
		g, gerr := buildGraph(root)
		if gerr != nil {
			return errln(gerr), 1
		}
		showOut := inv.Outbound || !inv.Inbound
		showIn := inv.Inbound || !inv.Outbound
		out := g.Forward(inv.Selector)
		in := g.Backlinks(inv.Selector)
		if inv.FilesOnly {
			paths := linkedPaths(out, in, showOut, showIn)
			if inv.JSON {
				return mustJSON(paths), 0
			}
			return formatPaths(paths, pal), 0
		}
		if inv.JSON {
			p := linksJSON{Path: inv.Selector}
			if showOut {
				p.Outbound = out
			}
			if showIn {
				p.Inbound = in
			}
			return mustJSON(p), 0
		}
		return formatLinks(inv.Selector, out, in, showOut, showIn, pal), 0

	case ModeTags:
		if inv.Selector == "" {
			tags, terr := query.Tags(root)
			if terr != nil {
				return errln(terr), 1
			}
			if inv.JSON {
				return mustJSON(tags), 0
			}
			return formatTagsIndex(tags, pal), 0
		}
		groups, terr := query.TagGroups(root, inv.Selector)
		if terr != nil {
			return errln(terr), 1
		}
		if inv.JSON {
			return mustJSON(groups), 0
		}
		return formatTagGroups(groups, pal), 0

	case ModeTypes:
		types, terr := query.Types(root)
		if terr != nil {
			return errln(terr), 1
		}
		if inv.JSON {
			return mustJSON(types), 0
		}
		return formatTypesIndex(types, pal), 0

	case ModeNext:
		g, gerr := buildGraph(root)
		if gerr != nil {
			return errln(gerr), 1
		}
		ready, _ := g.Next(graph.DefaultNextOptions())
		ready = filterReady(ready, inv)
		if inv.Limit > 0 && len(ready) > inv.Limit {
			ready = ready[:inv.Limit]
		}
		if inv.JSON {
			return mustJSON(ready), 0
		}
		items := make([]query.ListItem, len(ready))
		for i, r := range ready {
			items[i] = query.Summarize(r)
		}
		return formatList(items, pal), 0

	case ModeNew:
		if inv.Selector == "" {
			return needSelector("--new")
		}
		tmpl := layout.TemplateNote
		if inv.Type == "task" {
			tmpl = layout.TemplateTask
		}
		fields := map[string]string{}
		for k, v := range inv.Fields {
			fields[k] = v
		}
		if inv.Type != "" {
			fields["type"] = inv.Type // explicit --type wins over the template default
		}
		if inv.Status != "" {
			fields["status"] = inv.Status
		}
		res, nerr := store.New(root, tmpl, inv.Selector, fields, time.Now(), layout.DefaultConfig())
		if nerr != nil {
			return errln(nerr), 1
		}
		if len(inv.Tags) > 0 {
			if _, serr := store.Set(root, inv.Selector, nil, inv.Tags); serr != nil {
				return errln(serr), 1
			}
		}
		if inv.JSON {
			return mustJSON(res), 0
		}
		return formatWrite(res, pal), 0

	case ModeSet:
		if inv.Selector == "" {
			return needSelector("--set")
		}
		fields := map[string]string{}
		for k, v := range inv.Fields {
			fields[k] = v
		}
		if inv.Status != "" {
			fields["status"] = inv.Status
		}
		res, serr := store.Set(root, inv.Selector, fields, inv.Tags)
		if serr != nil {
			return errln(serr), 1
		}
		if inv.JSON {
			return mustJSON(res), 0
		}
		return formatWrite(res, pal), 0

	case ModeValidate:
		rep, verr := validate.Run(root, inv.Strict)
		if verr != nil {
			return errln(verr), 1
		}
		code := 0
		if !rep.OK {
			code = 1
		}
		if inv.JSON {
			return mustJSON(rep), code
		}
		return formatReport(rep, pal), code

	case ModeGraph:
		g, gerr := buildGraph(root)
		if gerr != nil {
			return errln(gerr), 1
		}
		edges := g.Edges()
		if inv.JSON {
			return mustJSON(edges), 0
		}
		return formatEdges(edges, pal), 0
	}
	return usage(), 2
}

// matcher compiles the search selector as an RE2 pattern for match
// highlighting. Returns nil when color is off or the pattern won't compile.
func matcher(inv Invocation, pal palette) *regexp.Regexp {
	if !pal.on || inv.Selector == "" {
		return nil
	}
	pat := inv.Selector
	if inv.IgnoreCase {
		pat = "(?i)" + pat
	}
	m, err := regexp.Compile(pat)
	if err != nil {
		return nil
	}
	return m
}

func dispatchList(root string, inv Invocation, pal palette) (string, int) {
	items, lerr := query.List(root, query.ListFilters{Status: inv.Status, Type: inv.Type, Tags: inv.Tags})
	if lerr != nil {
		return errln(lerr), 1
	}
	if inv.Selector != "" {
		items = filterItems(items, inv.Selector)
	}
	if inv.Limit > 0 && len(items) > inv.Limit {
		items = items[:inv.Limit]
	}
	if inv.JSON {
		return mustJSON(items), 0
	}
	return formatList(items, pal), 0
}

func filterItems(items []query.ListItem, sel string) []query.ListItem {
	s := strings.ToLower(sel)
	out := []query.ListItem{}
	for _, it := range items {
		if strings.Contains(strings.ToLower(it.Path), s) || strings.Contains(strings.ToLower(it.Title), s) {
			out = append(out, it)
		}
	}
	return out
}

func filterReady(ready []model.FileVitals, inv Invocation) []model.FileVitals {
	sel := strings.ToLower(inv.Selector)
	out := []model.FileVitals{}
	for _, r := range ready {
		it := query.Summarize(r)
		if inv.Type != "" && it.Type != inv.Type {
			continue
		}
		if inv.Status != "" && it.Status != inv.Status {
			continue
		}
		if len(inv.Tags) > 0 && !hasAllTags(it.Tags, inv.Tags) {
			continue
		}
		if sel != "" && !strings.Contains(strings.ToLower(it.Path), sel) && !strings.Contains(strings.ToLower(it.Title), sel) {
			continue
		}
		out = append(out, r)
	}
	return out
}

func hasAllTags(have, want []string) bool {
	set := map[string]bool{}
	for _, t := range have {
		set[t] = true
	}
	for _, w := range want {
		if !set[w] {
			return false
		}
	}
	return true
}

func needSelector(mode string) (string, int) {
	return "tfq: " + mode + " requires a selector\n\n" + usage(), 2
}

func buildGraph(dir string) (*graph.Graph, error) {
	recs, _, err := scan.Collect(dir)
	if err != nil {
		return nil, err
	}
	opts := graph.DefaultOptions()
	if path, ok := cueschema.Find(dir); ok {
		if s, lerr := cueschema.Load(path); lerr == nil {
			if efs := s.EdgeFields(); len(efs) > 0 {
				names := make([]string, len(efs))
				for i, e := range efs {
					names[i] = e.Name
				}
				opts = graph.Options{FrontmatterEdgeFields: names}
			}
		}
	}
	return graph.Build(recs, opts), nil
}

func errln(err error) string {
	fmt.Fprintln(os.Stderr, "tfq: "+err.Error())
	return ""
}

func mustJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(b)
}

func usage() string {
	return strings.Join([]string{
		"tfq — query frontmatter'd text records",
		"",
		"usage: tfq [OPTIONS] [SELECTOR...]",
		"",
		"Default (search):",
		"  tfq battery supply        search records",
		"  tfq -i battery            case-insensitive search",
		"  tfq -l battery            matching files only",
		"  tfq --status pending      list records (empty selector)",
		"",
		"Modes (one at a time):",
		"  --list [QUERY]            record summaries",
		"  --show REF                show one record (--raw, --frontmatter)",
		"  --links REF               outbound + inbound links (--inbound/--outbound)",
		"  --tags [QUERY]            tag index / tag search",
		"  --types                   frontmatter type: index",
		"  --next [QUERY]            ready tasks (deps satisfied)",
		"  --new SLUG                create record (--type, --tag, --status, --field)",
		"  --set REF                 update record (--status, --tag, --field)",
		"  --done REF                mark task done",
		"  --validate                validate collection (--strict)",
		"  --inspect FILE            FileVitals for one file",
		"  --graph                   all resolved edges",
		"  --version  --help",
		"",
		"Aliases: --task (=--new --type task), --backlinks (=--links --inbound),",
		"         --outlinks/--forward-links (=--links --outbound)",
		"",
		"Filters:  --type T (frontmatter type:)   --tag T (repeatable)   --status S   --limit N",
		"Search:   -i/--ignore-case   -l/--files-with-matches   -c/--count   --heading/--no-heading",
		"          --in heading|tag|link   narrow matches to a structural element (repeatable)",
		"Root:     --root DIR   (else $TFQ_ROOT, then nearest ancestor with",
		"          .tfq.cue/.tfq.yaml/.tfq/, then the current directory)",
		"Output:   --json   --color auto|always|never   --no-color   -e/--query PATTERN",
	}, "\n")
}

func main() {
	isTTY := false
	if fi, err := os.Stdout.Stat(); err == nil {
		isTTY = fi.Mode()&os.ModeCharDevice != 0
	}
	out, code := runEnv(os.Args[1:], isTTY, os.Getenv("NO_COLOR") != "")
	if out != "" {
		if code == 0 {
			fmt.Println(out)
		} else {
			fmt.Fprintln(os.Stderr, out)
		}
	}
	os.Exit(code)
}
