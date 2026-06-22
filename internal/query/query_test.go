package query

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestListFilters(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "001.md", "---\nid: \"001\"\nstatus: pending\ntype: task\ntags: [a]\n---\n# One\n")
	writeFile(t, dir, "002.md", "---\nid: \"002\"\nstatus: completed\ntype: task\n---\n# Two\n")

	all, err := List(dir, ListFilters{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("want 2, got %d", len(all))
	}

	pend, _ := List(dir, ListFilters{Status: "pending"})
	if len(pend) != 1 || pend[0].Path != "001.md" || pend[0].Title != "One" {
		t.Errorf("status filter wrong: %#v", pend)
	}

	tagged, _ := List(dir, ListFilters{Tags: []string{"a"}})
	if len(tagged) != 1 || tagged[0].Path != "001.md" {
		t.Errorf("tag filter wrong: %#v", tagged)
	}
}

func TestRead(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "note.md", "---\nslug: hi\n---\n# Heading\nbody line\n")
	r, err := Read(dir, "hi")
	if err != nil {
		t.Fatal(err)
	}
	if r.Path != "note.md" {
		t.Errorf("path = %q", r.Path)
	}
	if r.Frontmatter["slug"] != "hi" {
		t.Errorf("frontmatter = %#v", r.Frontmatter)
	}
	if want := "# Heading\nbody line\n"; r.Body != want {
		t.Errorf("body = %q, want %q", r.Body, want)
	}
	if _, err := Read(dir, "nonexistent"); err == nil {
		t.Error("unknown ref should error")
	}
}

func TestListMultiTagAnd(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.md", "---\ntags: [x, y]\n---\n# a\n")
	writeFile(t, dir, "b.md", "---\ntags: [x]\n---\n# b\n")
	items, err := List(dir, ListFilters{Tags: []string{"x", "y"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Path != "a.md" {
		t.Errorf("multi-tag list got %#v, want only a.md", items)
	}
}

func TestTagsIndexCounts(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.md", "---\ntags: [x, y]\n---\n# a\n")
	writeFile(t, dir, "b.md", "---\ntags: [x]\n---\n# b\n")
	tags, err := Tags(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 || tags[0].Tag != "x" || tags[0].Count != 2 {
		t.Errorf("tags index got %#v, want x=2 first", tags)
	}
}

func TestTagGroupsFilterAndMembers(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.md", "---\ntags: [supply-chain]\n---\n# a\n")
	writeFile(t, dir, "b.md", "---\ntags: [risk]\n---\n# b\n")
	groups, err := TagGroups(dir, "supply")
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].Tag != "supply-chain" || len(groups[0].Records) != 1 {
		t.Errorf("tag groups got %#v, want one supply-chain group with 1 record", groups)
	}
}
