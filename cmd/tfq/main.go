package main

import (
	"encoding/json"
	"fmt"
	"os"

	"tfq/internal/engine"
	"tfq/internal/graph"
	"tfq/internal/scan"
	"tfq/internal/search"
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
	default:
		return usage(), 2
	}
}

func buildGraph(dir string) (*graph.Graph, error) {
	recs, _, err := scan.Collect(dir)
	if err != nil {
		return nil, err
	}
	return graph.Build(recs, graph.DefaultOptions()), nil
}

func mustJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(b)
}

func usage() string {
	return "usage: tfq <inspect <file> | graph <dir> | backlinks <ref> <dir> | next <dir> | search <query> <dir>>"
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
