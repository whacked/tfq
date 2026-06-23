package search

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

func TestSearchPlain(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.md", "---\ntype: note\n---\nhello world\nfoo bar\n")
	writeFile(t, dir, "b.md", "---\ntype: log\n---\nhello again\n")

	hits, _, err := Search(dir, "hello", Filters{})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d: %#v", len(hits), hits)
	}
	if hits[0].Path != "a.md" || hits[0].Line != 4 {
		t.Errorf("hit0 = %#v", hits[0])
	}
}

func TestSearchTypeFilter(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.md", "---\ntype: note\n---\nhello world\n")
	writeFile(t, dir, "b.md", "---\ntype: log\n---\nhello again\n")

	hits, _, err := Search(dir, "hello", Filters{Type: "log"})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 || hits[0].Path != "b.md" {
		t.Errorf("type filter wrong: %#v", hits)
	}
}

func TestSearchNoMatches(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.md", "nothing here\n")
	hits, _, err := Search(dir, "zzzznomatch", Filters{})
	if err != nil {
		t.Fatalf("no-match must not error: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("expected 0 hits, got %#v", hits)
	}
}

func TestSearchIgnoreCase(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.md", "Needle here\n")
	hits, _, err := Search(dir, "needle", Filters{IgnoreCase: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("ignore-case search got %d hits, want 1", len(hits))
	}
}

func TestSearchMultiTagAnd(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.md", "---\ntags: [x, y]\n---\nhello\n")
	writeFile(t, dir, "b.md", "---\ntags: [x]\n---\nhello\n")
	hits, _, _ := Search(dir, "hello", Filters{Tags: []string{"x", "y"}})
	if len(hits) != 1 || hits[0].Path != "a.md" {
		t.Errorf("multi-tag AND got %#v, want only a.md", hits)
	}
}

func TestSearchStatusFilter(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.md", "---\nstatus: done\n---\nhello\n")
	writeFile(t, dir, "b.md", "---\nstatus: pending\n---\nhello\n")
	hits, _, _ := Search(dir, "hello", Filters{Status: "pending"})
	if len(hits) != 1 || hits[0].Path != "b.md" {
		t.Errorf("status filter got %#v, want only b.md", hits)
	}
}

func TestSearchInHeading(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.md", "# Battery supply\nbattery in prose\n")
	hits, _, _ := Search(dir, "battery", Filters{IgnoreCase: true, In: []string{"heading"}})
	if len(hits) != 1 || hits[0].Line != 1 {
		t.Fatalf("--in heading should keep only the heading line, got %#v", hits)
	}
}

func TestSearchInTag(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.md", "tracking #battery rollout\nbattery in prose\n")
	hits, _, _ := Search(dir, "battery", Filters{In: []string{"tag"}})
	if len(hits) != 1 || hits[0].Line != 1 {
		t.Fatalf("--in tag should keep only the #battery line, got %#v", hits)
	}
}

func TestSearchInLink(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.md", "see [[battery-spec]] here\nbattery in prose\n")
	hits, _, _ := Search(dir, "battery", Filters{In: []string{"link"}})
	if len(hits) != 1 || hits[0].Line != 1 {
		t.Fatalf("--in link should keep only the link line, got %#v", hits)
	}
}

func TestSearchKindsPopulated(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.md", "# Battery notes\njust battery prose\n")
	hits, _, _ := Search(dir, "battery", Filters{IgnoreCase: true})
	byLine := map[int][]string{}
	for _, h := range hits {
		byLine[h.Line] = h.Kinds
	}
	if len(byLine[1]) != 1 || byLine[1][0] != "heading" {
		t.Errorf("line 1 kinds = %#v, want [heading]", byLine[1])
	}
	if len(byLine[2]) != 0 {
		t.Errorf("prose line kinds = %#v, want empty", byLine[2])
	}
}
