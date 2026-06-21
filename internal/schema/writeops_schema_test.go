package schema

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"tfq/internal/layout"
	"tfq/internal/query"
	"tfq/internal/store"
)

func TestWriteOpsSchemas(t *testing.T) {
	root := t.TempDir()
	d, _ := time.Parse("2006-01-02", "2026-06-22")

	w, err := store.New(root, layout.TemplateTask, "do-it", nil, d, layout.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateWrite(w); err != nil {
		t.Errorf("write schema violation: %v", err)
	}

	items, err := query.List(root, query.ListFilters{})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateList(items); err != nil {
		t.Errorf("list schema violation: %v", err)
	}

	_ = os.WriteFile(filepath.Join(root, "extra.md"), []byte("---\nslug: e\n---\nbody\n"), 0o644)
	rec, err := query.Read(root, "e")
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateRecord(rec); err != nil {
		t.Errorf("record schema violation: %v", err)
	}
}
