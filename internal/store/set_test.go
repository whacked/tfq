package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tfq/internal/graph"
	"tfq/internal/scan"
)

func TestSetStatusAndTag(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "001.md")
	if err := os.WriteFile(p, []byte("---\nid: \"001\"\nstatus: pending\ntags: [a]\n---\n# body\nkeep me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Set(root, "001", map[string]string{"status": "completed"}, []string{"reviewed"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != "001.md" || res.Action != "updated" {
		t.Fatalf("result = %#v", res)
	}
	b, _ := os.ReadFile(p)
	s := string(b)
	if !strings.Contains(s, "status: completed") {
		t.Errorf("status not updated:\n%s", s)
	}
	if !strings.Contains(s, "reviewed") {
		t.Errorf("tag not appended:\n%s", s)
	}
	if !strings.Contains(s, "# body\nkeep me") {
		t.Errorf("body not preserved:\n%s", s)
	}
	if !strings.Contains(s, "id:") {
		t.Errorf("id key lost:\n%s", s)
	}
}

func TestSetAddsMissingKey(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "n.md")
	if err := os.WriteFile(p, []byte("---\nslug: n\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Set(root, "n", map[string]string{"status": "active"}, nil); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "status: active") {
		t.Errorf("missing key not added:\n%s", string(b))
	}
}

func TestSetUnknownRef(t *testing.T) {
	root := t.TempDir()
	if _, err := Set(root, "ghost", map[string]string{"x": "y"}, nil); err == nil {
		t.Error("unknown ref should error")
	}
}

func TestSetAmbiguousRefIsError(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.md"), []byte("---\nslug: dup\nstatus: pending\n---\n# a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.md"), []byte("---\ntitle: dup\nstatus: pending\n---\n# b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Set(root, "dup", map[string]string{"status": "done"}, nil)
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguous-reference error, got %v", err)
	}
}

func TestSetWithWritesDependencyList(t *testing.T) {
	root := t.TempDir()
	for _, id := range []string{"001", "002", "003"} {
		p := filepath.Join(root, id+".md")
		if err := os.WriteFile(p, []byte("---\nid: \""+id+"\"\nstatus: pending\n---\n# "+id+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := SetWith(root, "001", nil, nil, map[string][]string{"dependencies": {"002", "003"}}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(root, "001.md"))
	s := string(b)
	if !strings.Contains(s, "002") || !strings.Contains(s, "003") {
		t.Errorf("both deps should be written:\n%s", s)
	}
	// resolves as two distinct blocking edges, not one "002,003" scalar
	recs, _, err := scan.Collect(root)
	if err != nil {
		t.Fatal(err)
	}
	g := graph.Build(recs, graph.DefaultOptions())
	out := g.Forward("001")
	if len(out) != 2 {
		t.Errorf("expected 2 resolved dependency edges, got %d: %#v", len(out), out)
	}
}
