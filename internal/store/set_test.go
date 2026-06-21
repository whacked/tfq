package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
