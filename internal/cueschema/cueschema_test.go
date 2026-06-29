package cueschema

import (
	"path/filepath"
	"testing"
)

func TestLoadAndEdgeFields(t *testing.T) {
	s, err := Load(filepath.Join("testdata", ".tfq.cue"))
	if err != nil {
		t.Fatal(err)
	}
	efs := s.EdgeFields()
	got := map[string]bool{}
	for _, e := range efs {
		got[e.Name] = e.Blocking
	}
	if b, ok := got["dependencies"]; !ok || !b {
		t.Errorf("dependencies should be a blocking edge: %#v", efs)
	}
	if b, ok := got["parent"]; !ok || b {
		t.Errorf("parent should be a non-blocking edge: %#v", efs)
	}
	if _, ok := got["status"]; ok {
		t.Errorf("status is not an edge field: %#v", efs)
	}
}

func TestFind(t *testing.T) {
	got, ok := Find("testdata")
	if !ok {
		t.Fatal("expected to find .tfq.cue under testdata")
	}
	if filepath.Base(got) != ".tfq.cue" {
		t.Errorf("Find returned %q", got)
	}
}

func TestLoadCompileError(t *testing.T) {
	if _, err := Load("testdata/does-not-exist.cue"); err == nil {
		t.Error("expected error loading missing file")
	}
}

// A schema may live in a ```cue fenced block inside a markdown template
// (agent-resources keeps schemas in *.cue.template.md). Load must extract it.
func TestLoadExtractsCueFromMarkdown(t *testing.T) {
	s, err := Load("testdata/notes.template.md")
	if err != nil {
		t.Fatalf("load markdown template: %v", err)
	}
	valid := map[string]any{"date": "2026-06-30", "author": "agent", "slug": "ok-slug"}
	if vs := s.Validate(valid); len(vs) != 0 {
		t.Errorf("valid frontmatter should pass, got %#v", vs)
	}
	bad := map[string]any{"date": "nope", "author": "agent", "slug": "ok-slug"}
	if vs := s.Validate(bad); len(vs) == 0 {
		t.Error("bad date should fail the extracted schema")
	}
}
