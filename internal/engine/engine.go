package engine

import (
	"os"
	"path/filepath"

	"tfq/internal/extract"
	"tfq/internal/model"
	"tfq/internal/registry"
)

// InspectContent runs all extractors over already-loaded content. Pure (no I/O).
// All slices/maps in the result are guaranteed non-nil.
func InspectContent(path, content string) model.FileVitals {
	ext := filepath.Ext(path)
	format := registry.FormatFor(ext)

	fm, body, warns := extract.Frontmatter(content)
	headings, hw := extract.Headings(body, format)
	markers, mw := extract.Markers(body, format)
	links, lw := extract.Links(body)

	allWarn := []model.Warning{}
	allWarn = append(allWarn, warns...)
	allWarn = append(allWarn, hw...)
	allWarn = append(allWarn, mw...)
	allWarn = append(allWarn, lw...)

	if fm == nil {
		fm = map[string]any{}
	}
	if headings == nil {
		headings = []model.Heading{}
	}
	if links == nil {
		links = []model.Link{}
	}
	if markers == nil {
		markers = []model.Marker{}
	}

	return model.FileVitals{
		Path:        path,
		Ext:         ext,
		Format:      format,
		Frontmatter: fm,
		Headings:    headings,
		Links:       links,
		Markers:     markers,
		Warnings:    allWarn,
	}
}

// Inspect reads path and returns its FileVitals. Only I/O errors propagate.
func Inspect(path string) (model.FileVitals, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return model.FileVitals{}, err
	}
	return InspectContent(path, string(b)), nil
}
