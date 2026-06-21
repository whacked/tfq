package graph

import (
	"testing"

	"tfq/internal/model"
)

func recPath(path string, fm map[string]any, links ...model.Link) model.FileVitals {
	return model.FileVitals{
		Path: path, Format: "markdown",
		Frontmatter: fm, Headings: []model.Heading{},
		Links: links, Markers: []model.Marker{}, Warnings: []model.Warning{},
	}
}

func TestResolveByKeys(t *testing.T) {
	recs := []model.FileVitals{
		recPath("notes/note-a.md", map[string]any{"slug": "alpha"}),
		recPath("tasks/001-review.md", map[string]any{"id": "001"}),
	}
	g := Build(recs, DefaultOptions())

	cases := map[string]string{
		"notes/note-a.md": "notes/note-a.md",     // exact path
		"note-a":          "notes/note-a.md",     // basename
		"alpha":           "notes/note-a.md",     // slug
		"001":             "tasks/001-review.md", // id
		"001-review":      "tasks/001-review.md", // basename
	}
	for ref, want := range cases {
		got, ok := g.Resolve(ref)
		if !ok || got != want {
			t.Errorf("Resolve(%q) = %q ok=%v, want %q", ref, got, ok, want)
		}
	}
	if _, ok := g.Resolve("nonexistent"); ok {
		t.Errorf("expected nonexistent to not resolve")
	}
}

func TestResolveStripsSeqPrefix(t *testing.T) {
	// a task file NNN-slug.md should resolve by its bare slug too
	recs := []model.FileVitals{
		recPath("2026/06/001-do-thing.md", map[string]any{"id": "001"}),
	}
	g := Build(recs, DefaultOptions())
	got, ok := g.Resolve("do-thing")
	if !ok || got != "2026/06/001-do-thing.md" {
		t.Errorf("Resolve(do-thing) = %q ok=%v", got, ok)
	}
}

func TestEdgesResolveAndDangle(t *testing.T) {
	a := recPath("a.md", map[string]any{"slug": "a"},
		model.Link{Kind: model.LinkWiki, Target: "b", Line: 1, Col: 1})
	b := recPath("b.md", map[string]any{
		"slug":         "b",
		"dependencies": []any{"a"},
		"parent":       "ghost",
	})
	g := Build([]model.FileVitals{a, b}, DefaultOptions())

	var wikiTo, depTo, parentTo string
	parentSeen := false
	for _, e := range g.Edges() {
		switch {
		case e.From == "a.md" && e.Kind == "wiki":
			wikiTo = e.To
		case e.From == "b.md" && e.Kind == "fm:dependencies":
			depTo = e.To
		case e.From == "b.md" && e.Kind == "fm:parent":
			parentTo = e.To
			parentSeen = true
		}
	}
	if wikiTo != "b.md" {
		t.Errorf("wiki a->b resolved to %q", wikiTo)
	}
	if depTo != "a.md" {
		t.Errorf("dep b->a resolved to %q", depTo)
	}
	if !parentSeen || parentTo != "" {
		t.Errorf("parent ghost should dangle (To==\"\"), seen=%v to=%q", parentSeen, parentTo)
	}
	if len(g.Warnings()) == 0 {
		t.Errorf("expected a dangling-edge warning for parent: ghost")
	}
}
