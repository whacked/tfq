package graph

import (
	"testing"

	"tfq/internal/model"
)

func taskRec(id, status string, deps ...string) model.FileVitals {
	fm := map[string]any{"id": id, "status": status}
	if len(deps) > 0 {
		d := make([]any, len(deps))
		for i, x := range deps {
			d[i] = x
		}
		fm["dependencies"] = d
	}
	return model.FileVitals{
		Path: id + ".md", Format: "markdown", Frontmatter: fm,
		Headings: []model.Heading{}, Links: []model.Link{},
		Markers: []model.Marker{}, Warnings: []model.Warning{},
	}
}

func TestNextRespectsBlocking(t *testing.T) {
	// 001 done; 002 depends on 001 (ready); 003 depends on 002 (blocked); note has no status
	recs := []model.FileVitals{
		taskRec("001", "completed"),
		taskRec("002", "pending", "001"),
		taskRec("003", "pending", "002"),
		{Path: "note.md", Format: "markdown", Frontmatter: map[string]any{"slug": "note"},
			Headings: []model.Heading{}, Links: []model.Link{}, Markers: []model.Marker{}, Warnings: []model.Warning{}},
	}
	g := Build(recs, DefaultOptions())
	ready, _ := g.Next(DefaultNextOptions())

	if len(ready) != 1 {
		t.Fatalf("expected 1 ready task, got %d: %#v", len(ready), ready)
	}
	if ready[0].Path != "002.md" {
		t.Errorf("ready task = %q, want 002.md", ready[0].Path)
	}
}

func TestNextUnresolvedDepBlocksWithWarning(t *testing.T) {
	recs := []model.FileVitals{taskRec("005", "pending", "ghost")}
	g := Build(recs, DefaultOptions())
	ready, warns := g.Next(DefaultNextOptions())
	if len(ready) != 0 {
		t.Errorf("task with unresolved dep should be blocked, got %#v", ready)
	}
	if len(warns) == 0 {
		t.Errorf("expected a warning for unresolved dependency")
	}
}
