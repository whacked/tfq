package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustWrite(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
func contains(s, sub string) bool { return strings.Contains(s, sub) }

func TestRunHelpAndVersion(t *testing.T) {
	if out, code := run([]string{}); code != 0 || !contains(out, "usage") {
		t.Errorf("bare tfq should print help, got code=%d", code)
	}
	if out, code := run([]string{"--version"}); code != 0 || out != version {
		t.Errorf("--version = %q code=%d", out, code)
	}
	if _, code := run([]string{"--bogus"}); code != 2 {
		t.Errorf("unknown flag should exit 2, got %d", code)
	}
	if _, code := run([]string{"--show", "--links", "x"}); code != 2 {
		t.Errorf("two modes should exit 2, got %d", code)
	}
}

func TestRunSearchHumanAndJSON(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "a.md", "needle here\n")

	out, code := run([]string{"--root", dir, "needle"})
	if code != 0 || !contains(out, "a.md") || !contains(out, "1: needle here") {
		t.Errorf("human search wrong: code=%d\n%s", code, out)
	}

	out, code = run([]string{"--root", dir, "needle", "--json"})
	if code != 0 {
		t.Fatalf("json search exit %d: %s", code, out)
	}
	var hits []map[string]any
	if err := json.Unmarshal([]byte(out), &hits); err != nil || len(hits) != 1 {
		t.Errorf("json search wrong: %v\n%s", err, out)
	}

	out, _ = run([]string{"--root", dir, "needle", "-l"})
	if strings.TrimSpace(out) != "a.md" {
		t.Errorf("-l output = %q", out)
	}
}

func TestRunListFolding(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "a.md", "---\nstatus: pending\n---\n# a\n")
	mustWrite(t, dir, "b.md", "---\nstatus: done\n---\n# b\n")

	// empty selector + filter => list behavior
	out, code := run([]string{"--root", dir, "--status", "pending"})
	if code != 0 || !contains(out, "a.md") || contains(out, "b.md") {
		t.Errorf("empty-selector list wrong: %s", out)
	}
	// explicit --list mode
	out, _ = run([]string{"--root", dir, "--list"})
	if !contains(out, "a.md") || !contains(out, "b.md") {
		t.Errorf("--list wrong: %s", out)
	}
}

func TestRunTagsMode(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "a.md", "---\ntags: [battery, supply-chain]\n---\n# a\n")
	mustWrite(t, dir, "b.md", "---\ntags: [battery]\n---\n# b\n")

	out, code := run([]string{"--root", dir, "--tags"})
	if code != 0 || !contains(out, "battery") {
		t.Errorf("tags index wrong: %s", out)
	}
	out, _ = run([]string{"--root", dir, "--tags", "supply"})
	if !contains(out, "supply-chain") || !contains(out, "a.md") {
		t.Errorf("tags search wrong: %s", out)
	}
}

func TestRunLinksBothDirections(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "a.md", "---\nslug: a\n---\nsee [[b]]\n")
	mustWrite(t, dir, "b.md", "---\nslug: b\n---\nlink [[a]]\n")

	out, code := run([]string{"--root", dir, "--links", "a"})
	if code != 0 || !contains(out, "outbound") || !contains(out, "inbound") {
		t.Errorf("links both dirs wrong: %s", out)
	}
	out, _ = run([]string{"--root", dir, "--backlinks", "a"})
	if contains(out, "outbound") {
		t.Errorf("--backlinks should be inbound only: %s", out)
	}
}

func TestRunWriteWorkflow(t *testing.T) {
	dir := t.TempDir()

	// create two tasks
	if _, code := run([]string{"--root", dir, "--new", "first", "--type", "task"}); code != 0 {
		t.Fatal("new first failed")
	}
	if _, code := run([]string{"--root", dir, "--new", "second", "--type", "task"}); code != 0 {
		t.Fatal("new second failed")
	}
	// second depends on first
	if _, code := run([]string{"--root", dir, "--set", "second", "--field", "dependencies=first"}); code != 0 {
		t.Fatal("set dep failed")
	}
	// next gates "second" until "first" is done; "first" is ready
	out, _ := run([]string{"--root", dir, "--next"})
	if !contains(out, "first") || contains(out, "second") {
		t.Errorf("next gating wrong: %s", out)
	}
	// complete first via --done
	if _, code := run([]string{"--root", dir, "--done", "first"}); code != 0 {
		t.Fatal("done failed")
	}
	// now second is ready
	out, _ = run([]string{"--root", dir, "--next"})
	if !contains(out, "second") {
		t.Errorf("second should be ready: %s", out)
	}
	// show --raw prints the body
	out, code := run([]string{"--root", dir, "--show", "second", "--raw"})
	if code != 0 || !contains(out, "second") {
		t.Errorf("show --raw wrong: %s", out)
	}
}

func TestRunAmbiguousWriteErrors(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "a.md", "---\nslug: dup\nstatus: pending\n---\n# a\n")
	mustWrite(t, dir, "b.md", "---\ntitle: dup\nstatus: pending\n---\n# b\n")
	if _, code := run([]string{"--root", dir, "--done", "dup"}); code != 1 {
		t.Errorf("ambiguous write should exit 1, got %d", code)
	}
}

func TestRunInspectAndValidate(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, ".tfq.cue", "status: \"pending\" | \"completed\"\n")
	mustWrite(t, dir, "ok.md", "---\nstatus: completed\n---\n# ok\n")

	f := filepath.Join(dir, "ok.md")
	out, code := run([]string{"--inspect", f, "--json"})
	if code != 0 {
		t.Fatalf("inspect exit %d: %s", code, out)
	}
	var fv map[string]any
	if json.Unmarshal([]byte(out), &fv) != nil || fv["format"] != "markdown" {
		t.Errorf("inspect json wrong: %s", out)
	}

	if _, code := run([]string{"--root", dir, "--validate"}); code != 0 {
		t.Errorf("liberal validate should exit 0, got %d", code)
	}
	mustWrite(t, dir, "bad.md", "---\nstatus: nope\n---\n# bad\n")
	if _, code := run([]string{"--root", dir, "--validate", "--strict"}); code != 1 {
		t.Errorf("strict validate over bad record should exit 1, got %d", code)
	}
}

func TestRunInNarrowing(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, dir, "head.md", "# battery supply\nunrelated text\n")
	mustWrite(t, dir, "prose.md", "battery in prose here\n")
	out, code := run([]string{"--root", dir, "battery", "--in", "heading", "-l"})
	if code != 0 || strings.TrimSpace(out) != "head.md" {
		t.Errorf("--in heading -l should yield only head.md: code=%d out=%q", code, out)
	}
}

func TestRunValidateFileAgainstSchema(t *testing.T) {
	dir := t.TempDir()
	// schema lives in a markdown template, like agent-resources' *.cue.template.md
	mustWrite(t, dir, "notes.tpl.md", "```cue\ndate: string & =~\"^[0-9]{4}-[0-9]{2}-[0-9]{2}$\"\nslug: string & =~\"^[a-z0-9-]+$\"\n```\n")
	mustWrite(t, dir, "ok.md", "---\ndate: 2026-06-30\nslug: good-note\n---\n# ok\n")
	mustWrite(t, dir, "bad.md", "---\ndate: nope\nslug: Bad\n---\n# bad\n")

	tpl := filepath.Join(dir, "notes.tpl.md")
	ok := filepath.Join(dir, "ok.md")
	bad := filepath.Join(dir, "bad.md")

	if out, code := run([]string{"--validate", ok, "--schema", tpl}); code != 0 {
		t.Errorf("valid file should exit 0, got %d\n%s", code, out)
	}
	if _, code := run([]string{"--validate", bad, "--schema", tpl}); code != 1 {
		t.Errorf("invalid file should exit 1, got %d", code)
	}
}

func TestRunTypesAndNewType(t *testing.T) {
	dir := t.TempDir()
	if _, code := run([]string{"--root", dir, "--new", "job", "--type", "task"}); code != 0 {
		t.Fatal("new task failed")
	}
	// --type filter now finds the tfq-created task (type: task was written)
	out, code := run([]string{"--root", dir, "--type", "task"})
	if code != 0 || !contains(out, "job") {
		t.Errorf("--type task filter should find job: %s", out)
	}
	// --types lists the value via the types index
	out, code = run([]string{"--root", dir, "--types"})
	if code != 0 || !contains(out, "# types") || !contains(out, "task") {
		t.Errorf("--types should print the type index: code=%d %s", code, out)
	}
}
