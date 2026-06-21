package query

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"tfq/internal/graph"
	"tfq/internal/model"
	"tfq/internal/scan"
)

// ListItem is a compact record summary.
type ListItem struct {
	Path   string   `json:"path"`
	Title  string   `json:"title"`
	Status string   `json:"status"`
	Type   string   `json:"type"`
	Tags   []string `json:"tags"`
}

// ListFilters narrows a List (AND semantics; empty matches all).
type ListFilters struct {
	Status string
	Tag    string
	Type   string
}

func fmStr(fm map[string]any, key string) string {
	if s, ok := fm[key].(string); ok {
		return s
	}
	return ""
}

func fmTags(fm map[string]any) []string {
	out := []string{}
	switch t := fm["tags"].(type) {
	case []any:
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
	case []string:
		out = append(out, t...)
	}
	return out
}

func titleOf(r model.FileVitals) string {
	if t := fmStr(r.Frontmatter, "title"); t != "" {
		return t
	}
	if s := fmStr(r.Frontmatter, "slug"); s != "" {
		return s
	}
	if len(r.Headings) > 0 {
		return r.Headings[0].Text
	}
	return ""
}

// List returns filtered record summaries under root.
func List(root string, f ListFilters) ([]ListItem, error) {
	recs, _, err := scan.Collect(root)
	if err != nil {
		return nil, err
	}
	out := []ListItem{}
	for _, r := range recs {
		tags := fmTags(r.Frontmatter)
		if f.Status != "" && fmStr(r.Frontmatter, "status") != f.Status {
			continue
		}
		if f.Type != "" && fmStr(r.Frontmatter, "type") != f.Type {
			continue
		}
		if f.Tag != "" && !containsStr(tags, f.Tag) {
			continue
		}
		out = append(out, ListItem{
			Path:   r.Path,
			Title:  titleOf(r),
			Status: fmStr(r.Frontmatter, "status"),
			Type:   fmStr(r.Frontmatter, "type"),
			Tags:   tags,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func containsStr(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// Record is a full record including body.
type Record struct {
	Path        string         `json:"path"`
	Format      string         `json:"format"`
	Frontmatter map[string]any `json:"frontmatter"`
	Body        string         `json:"body"`
}

// Read resolves ref under root and returns the record with its body.
func Read(root, ref string) (Record, error) {
	recs, _, err := scan.Collect(root)
	if err != nil {
		return Record{}, err
	}
	g := graph.Build(recs, graph.DefaultOptions())
	rel, ok := g.Resolve(ref)
	if !ok {
		return Record{}, fmt.Errorf("no record matches %q", ref)
	}
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return Record{}, err
	}
	content := string(b)
	var target model.FileVitals
	for _, r := range recs {
		if r.Path == rel {
			target = r
			break
		}
	}
	return Record{Path: rel, Format: target.Format, Frontmatter: target.Frontmatter, Body: bodyAfterFrontmatter(content)}, nil
}

// bodyAfterFrontmatter returns content after a leading --- ... --- block.
func bodyAfterFrontmatter(content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r") != "---" {
		return content
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r") == "---" {
			return strings.Join(lines[i+1:], "\n")
		}
	}
	return content
}
