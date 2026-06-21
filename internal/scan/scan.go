package scan

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"tfq/internal/engine"
	"tfq/internal/model"
	"tfq/internal/registry"
)

// Collect walks root and inspects every file whose extension maps to a known
// document format (not "text"). Paths in the returned records are relative to
// root, slash-separated. Unreadable files become warnings, not errors.
func Collect(root string) ([]model.FileVitals, []model.Warning, error) {
	var recs []model.FileVitals
	var warns []model.Warning

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path != root && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if registry.FormatFor(filepath.Ext(name)) == "text" {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			warns = append(warns, model.Warning{Module: "scan", Message: "cannot read " + path + ": " + rerr.Error()})
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)
		recs = append(recs, engine.InspectContent(rel, string(b)))
		return nil
	})
	if err != nil {
		return nil, warns, err
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].Path < recs[j].Path })
	return recs, warns, nil
}
