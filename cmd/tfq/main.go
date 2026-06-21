package main

import (
	"encoding/json"
	"fmt"
	"os"

	"tfq/internal/cueschema"
	"tfq/internal/engine"
	"tfq/internal/graph"
	"tfq/internal/scan"
	"tfq/internal/search"
	"tfq/internal/validate"
)

// run returns (stdoutText, exitCode). Kept pure for testing; main wires it to os.
func run(args []string) (string, int) {
	if len(args) < 1 {
		return usage(), 2
	}
	switch args[0] {
	case "inspect":
		if len(args) != 2 {
			return usage(), 2
		}
		fv, err := engine.Inspect(args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, "tfq: "+err.Error())
			return "", 1
		}
		return mustJSON(fv), 0
	case "graph":
		if len(args) != 2 {
			return usage(), 2
		}
		g, err := buildGraph(args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, "tfq: "+err.Error())
			return "", 1
		}
		return mustJSON(g.Edges()), 0
	case "backlinks":
		if len(args) != 3 {
			return usage(), 2
		}
		g, err := buildGraph(args[2])
		if err != nil {
			fmt.Fprintln(os.Stderr, "tfq: "+err.Error())
			return "", 1
		}
		return mustJSON(g.Backlinks(args[1])), 0
	case "next":
		if len(args) != 2 {
			return usage(), 2
		}
		g, err := buildGraph(args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, "tfq: "+err.Error())
			return "", 1
		}
		ready, _ := g.Next(graph.DefaultNextOptions())
		return mustJSON(ready), 0
	case "search":
		if len(args) != 3 {
			return usage(), 2
		}
		hits, _, err := search.Search(args[2], args[1], search.Filters{})
		if err != nil {
			fmt.Fprintln(os.Stderr, "tfq: "+err.Error())
			return "", 1
		}
		return mustJSON(hits), 0
	case "validate":
		if len(args) < 2 || len(args) > 3 {
			return usage(), 2
		}
		strict := false
		dir := args[1]
		if len(args) == 3 {
			if args[2] != "--strict" {
				return usage(), 2
			}
			strict = true
		}
		rep, err := validate.Run(dir, strict)
		if err != nil {
			fmt.Fprintln(os.Stderr, "tfq: "+err.Error())
			return "", 1
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

func mustJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(b)
}

func usage() string {
	return "usage: tfq <inspect <file> | graph <dir> | backlinks <ref> <dir> | next <dir> | search <query> <dir> | validate <dir> [--strict]>"
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
