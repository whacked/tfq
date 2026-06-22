package main

import (
	"os"

	"tfq/internal/rootdir"
)

// resolveRoot picks the collection root from --root, $TFQ_ROOT, an ancestor
// marker, or the working directory.
func resolveRoot(explicit string) (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return rootdir.Resolve(explicit, os.Getenv("TFQ_ROOT"), wd)
}
