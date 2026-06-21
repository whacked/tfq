package graph

import (
	"path"
	"regexp"
	"sort"
	"strings"

	"tfq/internal/model"
)

var seqPrefix = regexp.MustCompile(`^\d+-`)

// stripSeqPrefix removes a leading "NNN-" sequence prefix (task filenames).
func stripSeqPrefix(s string) string { return seqPrefix.ReplaceAllString(s, "") }

// Edge is a typed, possibly-dangling reference between records.
type Edge struct {
	From string `json:"from"`
	Kind string `json:"kind"`
	Raw  string `json:"raw"`
	To   string `json:"to"` // "" if unresolved
}

// Options configures which frontmatter fields are treated as edges.
type Options struct {
	FrontmatterEdgeFields []string
}

// DefaultOptions returns the default frontmatter edge fields.
func DefaultOptions() Options {
	return Options{FrontmatterEdgeFields: []string{"dependencies", "parent", "supersedes", "source_notes", "context"}}
}

// Graph is an in-memory typed graph over records.
type Graph struct {
	records []model.FileVitals
	byKey   map[string]string // key -> canonical record path (first writer wins)
	edges   []Edge
	warns   []model.Warning
}

func baseNoExt(p string) string {
	b := path.Base(p)
	if i := strings.LastIndex(b, "."); i > 0 {
		b = b[:i]
	}
	return b
}

func fmString(fm map[string]any, key string) (string, bool) {
	if v, ok := fm[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s, true
		}
	}
	return "", false
}

// Build indexes records by key and resolves all edges.
func Build(records []model.FileVitals, opts Options) *Graph {
	g := &Graph{records: records, byKey: map[string]string{}}

	addKey := func(key, p string) {
		if key == "" {
			return
		}
		if _, exists := g.byKey[key]; !exists {
			g.byKey[key] = p
		}
	}
	for _, r := range records {
		addKey(r.Path, r.Path)
		base := baseNoExt(r.Path)
		addKey(base, r.Path)
		if s := stripSeqPrefix(base); s != base {
			addKey(s, r.Path)
		}
		for _, fk := range []string{"id", "slug", "title"} {
			if s, ok := fmString(r.Frontmatter, fk); ok {
				addKey(s, r.Path)
			}
		}
	}

	resolveRaw := func(raw string) string {
		if p, ok := g.byKey[raw]; ok {
			return p
		}
		if p, ok := g.byKey[baseNoExt(raw)]; ok {
			return p
		}
		return ""
	}

	for _, r := range records {
		for _, l := range r.Links {
			to := resolveRaw(l.Target)
			g.edges = append(g.edges, Edge{From: r.Path, Kind: l.Kind, Raw: l.Target, To: to})
			if to == "" {
				g.warns = append(g.warns, model.Warning{Module: "graph", Message: r.Path + ": dangling " + l.Kind + " link -> " + l.Target})
			}
		}
		for _, field := range opts.FrontmatterEdgeFields {
			for _, raw := range edgeValues(r.Frontmatter[field]) {
				to := resolveRaw(raw)
				g.edges = append(g.edges, Edge{From: r.Path, Kind: "fm:" + field, Raw: raw, To: to})
				if to == "" {
					g.warns = append(g.warns, model.Warning{Module: "graph", Message: r.Path + ": dangling " + field + " -> " + raw})
				}
			}
		}
	}
	sort.SliceStable(g.edges, func(i, j int) bool {
		if g.edges[i].From != g.edges[j].From {
			return g.edges[i].From < g.edges[j].From
		}
		return g.edges[i].Raw < g.edges[j].Raw
	})
	return g
}

// edgeValues normalizes a frontmatter value into a list of raw edge targets.
func edgeValues(v any) []string {
	switch t := v.(type) {
	case string:
		if t != "" {
			return []string{t}
		}
	case []any:
		out := []string{}
		for _, e := range t {
			if s, ok := e.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return t
	}
	return nil
}

// Resolve maps a reference (by any key) to a canonical record path.
func (g *Graph) Resolve(ref string) (string, bool) {
	if p, ok := g.byKey[ref]; ok {
		return p, true
	}
	if p, ok := g.byKey[baseNoExt(ref)]; ok {
		return p, true
	}
	return "", false
}

// Backlinks returns sorted unique source paths whose edges resolve to ref.
func (g *Graph) Backlinks(ref string) []string {
	target, ok := g.Resolve(ref)
	if !ok {
		return []string{}
	}
	seen := map[string]bool{}
	out := []string{}
	for _, e := range g.edges {
		if e.To == target && !seen[e.From] {
			seen[e.From] = true
			out = append(out, e.From)
		}
	}
	sort.Strings(out)
	return out
}

// Forward returns edges originating from the record ref resolves to.
func (g *Graph) Forward(ref string) []Edge {
	src, ok := g.Resolve(ref)
	if !ok {
		return []Edge{}
	}
	out := []Edge{}
	for _, e := range g.edges {
		if e.From == src {
			out = append(out, e)
		}
	}
	return out
}

// Edges returns all edges (sorted).
func (g *Graph) Edges() []Edge { return g.edges }

// Warnings returns dangling-edge warnings.
func (g *Graph) Warnings() []model.Warning { return g.warns }
