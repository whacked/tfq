package rootdir

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveExplicitWins(t *testing.T) {
	got, err := Resolve("/x/explicit", "/x/env", "/x/cwd")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/x/explicit" {
		t.Errorf("got %q, want /x/explicit", got)
	}
}

func TestResolveEnvWhenNoExplicit(t *testing.T) {
	got, _ := Resolve("", "/x/env", "/x/cwd")
	if got != "/x/env" {
		t.Errorf("got %q, want /x/env", got)
	}
}

func TestResolveAncestorMarker(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".tfq.cue"), []byte("status: string\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	got, _ := Resolve("", "", sub)
	if got != root {
		t.Errorf("got %q, want %q", got, root)
	}
}

func TestResolveFallsBackToCwd(t *testing.T) {
	dir := t.TempDir()
	got, _ := Resolve("", "", dir)
	if got != dir {
		t.Errorf("got %q, want %q (cwd fallback)", got, dir)
	}
}
