// Package rootdir resolves a tfq collection root from explicit flag, env, an
// ancestor marker, or the working directory.
package rootdir

import (
	"os"
	"path/filepath"
)

// markers are files/dirs whose presence marks a collection root.
var markers = []string{".tfq.cue", ".tfq.yaml", ".tfq"}

// Resolve picks the collection root: explicit (--root), then env (TFQ_ROOT),
// then the nearest ancestor of startDir containing a marker, then startDir.
func Resolve(explicit, env, startDir string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if env != "" {
		return env, nil
	}
	dir := startDir
	for {
		for _, m := range markers {
			if _, err := os.Stat(filepath.Join(dir, m)); err == nil {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return startDir, nil
}
