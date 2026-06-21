package main

import (
	"encoding/json"
	"os"
	"path/filepath"
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
