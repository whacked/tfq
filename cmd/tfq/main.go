package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"tfq/internal/cueschema"
	"tfq/internal/engine"
	"tfq/internal/graph"
	"tfq/internal/layout"
	"tfq/internal/query"
	"tfq/internal/scan"
	"tfq/internal/search"
	"tfq/internal/store"
	"tfq/internal/validate"
)

// version is the build version, in the form yyyymmdd.<nth-commit-of-day>.<hash>.
// Overridden at build time via -ldflags "-X main.version=..." (see Makefile);
// defaults to "dev" for plain `go build`.
var version = "dev"

// run returns (stdoutText, exitCode). Kept pure for testing; main wires it to os.
func run(args []string) (string, int) {
	if len(args) < 1 {
		return usage(), 2
	}
	verb, rest := args[0], args[1:]
	switch verb {
	case "help":
		return usage(), 0
	case "version":
		return version, 0
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
		sf := search.Filters{Type: flags["type"]}
		if t := flags["tag"]; t != "" {
			sf.Tags = []string{t}
		}
		hits, _, serr := search.Search(pos[1], pos[0], sf)
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
	case "read":
		pos, flags, err := partition(rest, map[string]bool{"raw": true})
		if err != nil || len(pos) != 2 {
			return usage(), 2
		}
		rec, rerr := query.Read(pos[1], pos[0])
		if rerr != nil {
			return errln(rerr), 1
		}
		if flags["raw"] == "true" {
			return rec.Body, 0
		}
		return mustJSON(rec), 0
	case "list":
		pos, flags, err := partition(rest, nil)
		if err != nil || len(pos) != 1 {
			return usage(), 2
		}
		lf := query.ListFilters{Status: flags["status"], Type: flags["type"]}
		if t := flags["tag"]; t != "" {
			lf.Tags = []string{t}
		}
		items, lerr := query.List(pos[0], lf)
		if lerr != nil {
			return errln(lerr), 1
		}
		return mustJSON(items), 0
	case "new":
		pos, flags, fields, _, err := partitionMulti(rest, nil)
		if err != nil || len(pos) != 2 {
			return usage(), 2
		}
		tmpl := layout.Template(flags["template"])
		if tmpl == "" {
			tmpl = layout.TemplateNote
		}
		res, nerr := store.New(pos[1], tmpl, pos[0], fields, time.Now(), layout.DefaultConfig())
		if nerr != nil {
			return errln(nerr), 1
		}
		return mustJSON(res), 0
	case "set":
		pos, flags, fields, tags, err := partitionMulti(rest, nil)
		if err != nil || len(pos) != 2 {
			return usage(), 2
		}
		if s, ok := flags["status"]; ok {
			fields["status"] = s
		}
		res, serr := store.Set(pos[1], pos[0], fields, tags)
		if serr != nil {
			return errln(serr), 1
		}
		return mustJSON(res), 0
	default:
		return usage(), 2
	}
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
		"tfq — query frontmatter'd text files",
		"",
		"usage: tfq <verb> [args] [flags]",
		"",
		"  inspect <file>                    comprehensive FileVitals JSON for one file",
		"  read <ref> <dir> [--raw]          a record (frontmatter + body), or --raw body only",
		"  search <query> <dir> [--type T] [--tag G]   ripgrep search + frontmatter filters",
		"  list <dir> [--status S] [--tag T] [--type T]   record summaries, filtered",
		"  links <ref> <dir>                 outgoing edges from a record",
		"  backlinks <ref> <dir>             records that reference <ref>",
		"  graph <dir>                       all resolved edges in the collection",
		"  next <dir>                        tasks ready to work on (deps satisfied)",
		"  new <slug> <dir> [--template note|task] [--field k=v ...]   create a record",
		"  set <ref> <dir> [--status S] [--add-tag T ...] [--field k=v ...]   mutate frontmatter",
		"  validate <dir> [--strict]         validate vs .tfq.cue + edge resolution",
		"  version                           build version (yyyymmdd.n.hash)",
		"  help                              this message",
	}, "\n")
}

func main() {
	out, code := run(os.Args[1:])
	if out != "" {
		if code == 0 {
			fmt.Println(out)
		} else {
			fmt.Fprintln(os.Stderr, out)
		}
	}
	os.Exit(code)
}
