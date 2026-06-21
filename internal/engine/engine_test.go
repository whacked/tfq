package engine

import "testing"

func TestInspectMarkdown(t *testing.T) {
	fv, err := Inspect("testdata/note.md")
	if err != nil {
		t.Fatal(err)
	}
	if fv.Format != "markdown" || fv.Ext != ".md" {
		t.Errorf("format/ext wrong: %s %s", fv.Format, fv.Ext)
	}
	if fv.Frontmatter["title"] != "Bandgap Notes" {
		t.Errorf("frontmatter title: %v", fv.Frontmatter["title"])
	}
	if len(fv.Headings) != 1 || fv.Headings[0].Text != "Bandgap synthesis" {
		t.Errorf("headings: %#v", fv.Headings)
	}
	foundWiki, foundFollowup := false, false
	for _, l := range fv.Links {
		if l.Kind == "wiki" && l.Target == "../tasks/001-review" {
			foundWiki = true
		}
	}
	for _, m := range fv.Markers {
		if m.Kind == "hashtag" && m.Value == "followup" {
			foundFollowup = true
		}
	}
	if !foundWiki {
		t.Errorf("wiki link not found: %#v", fv.Links)
	}
	if !foundFollowup {
		t.Errorf("#followup not found: %#v", fv.Markers)
	}
}

func TestInspectOrgTags(t *testing.T) {
	fv, err := Inspect("testdata/tasks.org")
	if err != nil {
		t.Fatal(err)
	}
	if fv.Format != "org" {
		t.Errorf("format: %s", fv.Format)
	}
	found := false
	for _, m := range fv.Markers {
		if m.Kind == "org-tag" && m.Value == "urgent" {
			found = true
		}
	}
	if !found {
		t.Errorf("org tag urgent not found: %#v", fv.Markers)
	}
}

func TestInspectContentNeverNil(t *testing.T) {
	fv := InspectContent("empty.md", "")
	if fv.Frontmatter == nil || fv.Headings == nil || fv.Links == nil || fv.Markers == nil || fv.Warnings == nil {
		t.Errorf("nil slice/map in output: %#v", fv)
	}
}

func TestInspectMissingFile(t *testing.T) {
	if _, err := Inspect("testdata/does-not-exist.md"); err == nil {
		t.Error("expected I/O error for missing file")
	}
}
