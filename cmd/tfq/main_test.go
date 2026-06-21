package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunInspect(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "n.md")
	if err := os.WriteFile(f, []byte("---\ntitle: T\n---\n# H\n#tag\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, code := run([]string{"inspect", f})
	if code != 0 {
		t.Fatalf("exit %d, out=%s", code, out)
	}
	var fv map[string]any
	if err := json.Unmarshal([]byte(out), &fv); err != nil {
		t.Fatalf("output not json: %v\n%s", err, out)
	}
	if fv["format"] != "markdown" {
		t.Errorf("format = %v", fv["format"])
	}
}

func TestRunUsage(t *testing.T) {
	if _, code := run([]string{}); code != 2 {
		t.Errorf("expected exit 2 for no args, got %d", code)
	}
	if _, code := run([]string{"bogus"}); code != 2 {
		t.Errorf("expected exit 2 for unknown subcommand, got %d", code)
	}
}

func mustWrite(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
func contains(s, sub string) bool { return strings.Contains(s, sub) }

func TestRunBacklinksAndNext(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "001.md", "---\nid: \"001\"\nstatus: completed\n---\n# done\n")
	mustWrite(t, dir, "002.md", "---\nid: \"002\"\nstatus: pending\ndependencies: [\"001\"]\n---\n# go\nsee [[001]]\n")

	out, code := run([]string{"next", dir})
	if code != 0 {
		t.Fatalf("next exit %d: %s", code, out)
	}
	if !contains(out, "002.md") || contains(out, "001.md") {
		t.Errorf("next output wrong: %s", out)
	}

	out, code = run([]string{"backlinks", "001", dir})
	if code != 0 {
		t.Fatalf("backlinks exit %d: %s", code, out)
	}
	if !contains(out, "002.md") {
		t.Errorf("backlinks output wrong: %s", out)
	}
}

func TestRunSearch(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "a.md", "needle in here\n")
	out, code := run([]string{"search", "needle", dir})
	if code != 0 {
		t.Fatalf("search exit %d: %s", code, out)
	}
	if !contains(out, "a.md") {
		t.Errorf("search output wrong: %s", out)
	}
}
