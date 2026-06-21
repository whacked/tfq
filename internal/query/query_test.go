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

	tagged, _ := List(dir, ListFilters{Tag: "a"})
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
