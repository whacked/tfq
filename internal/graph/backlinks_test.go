package graph

import (
	"reflect"
	"testing"

	"tfq/internal/model"
)

func TestBacklinksAndForward(t *testing.T) {
	a := recPath("a.md", map[string]any{"slug": "a"},
		model.Link{Kind: model.LinkWiki, Target: "c", Line: 1, Col: 1})
	b := recPath("b.md", map[string]any{"slug": "b"},
		model.Link{Kind: model.LinkWiki, Target: "c", Line: 1, Col: 1})
	c := recPath("c.md", map[string]any{"slug": "c"})
	g := Build([]model.FileVitals{a, b, c}, DefaultOptions())

	bl := g.Backlinks("c")
	if !reflect.DeepEqual(bl, []string{"a.md", "b.md"}) {
		t.Errorf("Backlinks(c) = %#v, want [a.md b.md]", bl)
	}
	if got := g.Backlinks("a"); len(got) != 0 {
		t.Errorf("Backlinks(a) = %#v, want empty", got)
	}
	fwd := g.Forward("a.md")
	if len(fwd) != 1 || fwd[0].To != "c.md" {
		t.Errorf("Forward(a) = %#v", fwd)
	}
}
