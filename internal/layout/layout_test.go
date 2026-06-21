package layout

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func date(t *testing.T) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", "2026-06-22")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestRelPath(t *testing.T) {
	c := DefaultConfig()
	d := date(t)
	note, err := c.RelPath(TemplateNote, "my-slug", d, 1)
	if err != nil {
		t.Fatal(err)
	}
	if note != "2026/06/2026-06-22.001-my-slug.md" {
		t.Errorf("note path = %q", note)
	}
	task, err := c.RelPath(TemplateTask, "do-thing", d, 4)
	if err != nil {
		t.Fatal(err)
	}
	if task != "2026/06/004-do-thing.md" {
		t.Errorf("task path = %q", task)
	}
	if _, err := c.RelPath("bogus", "x", d, 1); err == nil {
		t.Error("unknown template should error")
	}
}

func TestNextSequenceDaily(t *testing.T) {
	root := t.TempDir()
	c := DefaultConfig()
	d := date(t)
	n, err := c.NextSequence(root, TemplateNote, d)
	if err != nil || n != 1 {
		t.Fatalf("empty daily seq = %d (%v)", n, err)
	}
	shard := filepath.Join(root, "2026", "06")
	if err := os.MkdirAll(shard, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"2026-06-22.001-a.md", "2026-06-22.002-b.md", "2026-06-21.005-old.md"} {
		if err := os.WriteFile(filepath.Join(shard, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	n, _ = c.NextSequence(root, TemplateNote, d)
	if n != 3 {
		t.Errorf("daily seq = %d, want 3 (yesterday's 005 must not count)", n)
	}
}

func TestNextSequenceGlobal(t *testing.T) {
	root := t.TempDir()
	c := DefaultConfig()
	d := date(t)
	shard := filepath.Join(root, "2026", "05")
	if err := os.MkdirAll(shard, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"003-x.md", "007-y.md"} {
		if err := os.WriteFile(filepath.Join(shard, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	n, _ := c.NextSequence(root, TemplateTask, d)
	if n != 8 {
		t.Errorf("global seq = %d, want 8", n)
	}
}
