package extract

import (
	"os"
	"testing"

	"tfq/internal/model"
)

func findLink(ls []model.Link, kind, target string) *model.Link {
	for i := range ls {
		if ls[i].Kind == kind && ls[i].Target == target {
			return &ls[i]
		}
	}
	return nil
}

func TestLinksCorpus(t *testing.T) {
	b, err := os.ReadFile("testdata/links_corpus.md")
	if err != nil {
		t.Fatal(err)
	}
	ls, _ := Links(string(b))

	if findLink(ls, model.LinkEmbed, "image.png") == nil {
		t.Errorf("missing embed image.png: %#v", ls)
	}
	if e := findLink(ls, model.LinkEmbed, "note"); e == nil || e.Label == nil || *e.Label != "Nice Note" {
		t.Errorf("embed alias wrong: %#v", ls)
	}
	if o := findLink(ls, model.LinkOrg, "https://o.example"); o == nil || o.Label == nil || *o.Label != "Org Desc" {
		t.Errorf("org link wrong: %#v", ls)
	}
	if findLink(ls, model.LinkWiki, "Plain Note") == nil {
		t.Errorf("missing wiki Plain Note: %#v", ls)
	}
	if w := findLink(ls, model.LinkWiki, "Target"); w == nil || w.Label == nil || *w.Label != "Shown" {
		t.Errorf("wiki alias wrong: %#v", ls)
	}
	if m := findLink(ls, model.LinkMarkdown, "https://md.example/page"); m == nil || m.Label == nil || *m.Label != "Click here" {
		t.Errorf("markdown link wrong: %#v", ls)
	}
	if findLink(ls, model.LinkAutolink, "https://auto.example") == nil {
		t.Errorf("missing autolink: %#v", ls)
	}
	if findLink(ls, model.LinkBareURL, "https://bare.example/x") == nil {
		t.Errorf("missing bare url: %#v", ls)
	}
	// the bare url inside the markdown link target must NOT be double-counted
	count := 0
	for _, l := range ls {
		if l.Target == "https://md.example/page" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("md target double counted: %d", count)
	}
}
