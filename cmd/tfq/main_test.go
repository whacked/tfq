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

func TestRunSearchFilters(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "a.md", "---\ntype: note\n---\nhello world\n")
	mustWrite(t, dir, "b.md", "---\ntype: log\n---\nhello again\n")

	out, code := run([]string{"search", "hello", dir, "--type", "log"})
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	if !contains(out, "b.md") || contains(out, "a.md") {
		t.Errorf("--type filter not applied: %s", out)
	}
}

func TestRunLinks(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "a.md", "---\nslug: a\n---\nsee [[b]]\n")
	mustWrite(t, dir, "b.md", "---\nslug: b\n---\n# b\n")

	out, code := run([]string{"links", "a", dir})
	if code != 0 {
		t.Fatalf("exit %d: %s", code, out)
	}
	if !contains(out, "\"to\": \"b.md\"") {
		t.Errorf("links should show forward edge to b.md: %s", out)
	}
}

func TestRunHelp(t *testing.T) {
	out, code := run([]string{"help"})
	if code != 0 {
		t.Errorf("help should exit 0, got %d", code)
	}
	for _, verb := range []string{"inspect", "search", "links", "backlinks", "graph", "next", "validate"} {
		if !contains(out, verb) {
			t.Errorf("help missing verb %q", verb)
		}
	}
}

func TestRunNewAndSetAndListAndRead(t *testing.T) {
	dir := t.TempDir()

	// new task
	out, code := run([]string{"new", "do-thing", dir, "--template", "task"})
	if code != 0 {
		t.Fatalf("new exit %d: %s", code, out)
	}
	if !contains(out, "\"action\": \"created\"") {
		t.Errorf("new output: %s", out)
	}

	// list shows it as pending
	out, code = run([]string{"list", dir, "--status", "pending"})
	if code != 0 {
		t.Fatalf("list exit %d: %s", code, out)
	}
	if !contains(out, "do-thing") {
		t.Errorf("list output: %s", out)
	}

	// set it to completed
	out, code = run([]string{"set", "do-thing", dir, "--status", "completed"})
	if code != 0 {
		t.Fatalf("set exit %d: %s", code, out)
	}
	if !contains(out, "\"action\": \"updated\"") {
		t.Errorf("set output: %s", out)
	}

	// now no pending tasks
	out, _ = run([]string{"list", dir, "--status", "pending"})
	if contains(out, "do-thing") {
		t.Errorf("task should no longer be pending: %s", out)
	}

	// read --raw shows the body
	out, code = run([]string{"read", "do-thing", dir, "--raw"})
	if code != 0 {
		t.Fatalf("read exit %d: %s", code, out)
	}
	if !contains(out, "do thing") {
		t.Errorf("read --raw output: %s", out)
	}
}

func TestRunValidate(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, ".tfq.cue", "status: \"pending\" | \"completed\"\n")
	mustWrite(t, dir, "ok.md", "---\nstatus: completed\n---\n# ok\n")

	out, code := run([]string{"validate", dir})
	if code != 0 {
		t.Fatalf("liberal validate should exit 0, got %d: %s", code, out)
	}
	if !contains(out, "\"ok\": true") {
		t.Errorf("expected ok:true: %s", out)
	}

	// strict over a bad record exits 1
	mustWrite(t, dir, "bad.md", "---\nstatus: nope\n---\n# bad\n")
	_, code = run([]string{"validate", dir, "--strict"})
	if code != 1 {
		t.Errorf("strict validate over bad record should exit 1, got %d", code)
	}
}
