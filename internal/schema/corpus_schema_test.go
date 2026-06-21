package schema

import (
	"os"
	"path/filepath"
	"testing"

	"tfq/internal/graph"
	"tfq/internal/scan"
	"tfq/internal/search"
)

func TestEdgesOutputMatchesSchema(t *testing.T) {
	recs, _, err := scan.Collect(filepath.Join("..", "scan", "testdata", "vault"))
	if err != nil {
		t.Fatal(err)
	}
	g := graph.Build(recs, graph.DefaultOptions())
	if err := ValidateEdges(g.Edges()); err != nil {
		t.Errorf("edges schema violation: %v", err)
	}
	// next output is []FileVitals; each must pass the FileVitals schema
	ready, _ := g.Next(graph.DefaultNextOptions())
	for _, r := range ready {
		if err := ValidateFileVitals(r); err != nil {
			t.Errorf("next item schema violation: %v", err)
		}
	}
}

func TestHitsOutputMatchesSchema(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.md"), []byte("hello there\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hits, _, err := search.Search(dir, "hello", search.Filters{})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateHits(hits); err != nil {
		t.Errorf("hits schema violation: %v", err)
	}
}
