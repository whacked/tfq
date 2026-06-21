package schema

import (
	"os"
	"path/filepath"
	"testing"

	"tfq/internal/engine"
)

func TestSchemaItself(t *testing.T) {
	if len(FileVitalsSchema) == 0 {
		t.Fatal("embedded schema is empty")
	}
}

func TestEngineOutputMatchesSchema(t *testing.T) {
	root := filepath.Join("..", "engine", "testdata")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read fixtures: %v", err)
	}
	checked := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		fv := engine.InspectContent(e.Name(), string(b))
		if err := ValidateFileVitals(fv); err != nil {
			t.Errorf("%s: schema violation: %v", e.Name(), err)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no fixtures checked")
	}
}

func TestValidateCatchesBadOutput(t *testing.T) {
	bad := `{"path":"x","ext":".md","format":"WRONG","frontmatter":{},"headings":[],"links":[],"markers":[],"warnings":[]}`
	if err := validateJSON([]byte(bad)); err == nil {
		t.Error("expected schema rejection for bad format enum")
	}
}
