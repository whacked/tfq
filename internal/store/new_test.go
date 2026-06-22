package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tfq/internal/layout"
)

func fixedDate(t *testing.T) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", "2026-06-22")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestNewNote(t *testing.T) {
	root := t.TempDir()
	res, err := New(root, layout.TemplateNote, "my-idea", nil, fixedDate(t), layout.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != "2026/06/2026-06-22.001-my-idea.md" || res.Action != "created" {
		t.Fatalf("result = %#v", res)
	}
	b, err := os.ReadFile(filepath.Join(root, res.Path))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "slug: my-idea") || !strings.Contains(s, "author: agent") {
		t.Errorf("note frontmatter wrong:\n%s", s)
	}
}

func TestNewTaskWithFields(t *testing.T) {
	root := t.TempDir()
	res, err := New(root, layout.TemplateTask, "do-thing", map[string]string{"priority": "high"}, fixedDate(t), layout.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != "2026/06/001-do-thing.md" {
		t.Fatalf("path = %q", res.Path)
	}
	b, _ := os.ReadFile(filepath.Join(root, res.Path))
	s := string(b)
	if !strings.Contains(s, "status: pending") || !strings.Contains(s, "priority: high") {
		t.Errorf("task frontmatter wrong:\n%s", s)
	}
	if !strings.Contains(s, "id: \"001\"") {
		t.Errorf("task id must be a quoted string (leading zero preserved):\n%s", s)
	}
}

func TestNewRejectsBadSlug(t *testing.T) {
	root := t.TempDir()
	if _, err := New(root, layout.TemplateNote, "Bad Slug", nil, fixedDate(t), layout.DefaultConfig()); err == nil {
		t.Error("expected error for invalid slug")
	}
}

func TestNewNoOverwrite(t *testing.T) {
	root := t.TempDir()
	cfg := layout.DefaultConfig()
	if _, err := New(root, layout.TemplateTask, "x", nil, fixedDate(t), cfg); err != nil {
		t.Fatal(err)
	}
	res, err := New(root, layout.TemplateTask, "y", nil, fixedDate(t), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != "2026/06/002-y.md" {
		t.Errorf("second task path = %q, want 002-y.md", res.Path)
	}
}
