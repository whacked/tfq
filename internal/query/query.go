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
	Tags   []string
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
		if !hasAllTags(tags, f.Tags) {
			continue
		}
		out = append(out, Summarize(r))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func hasAllTags(have, want []string) bool {
	for _, w := range want {
		if !containsStr(have, w) {
			return false
		}
	}
	return true
}

// Summarize builds the compact ListItem view of a record.
func Summarize(r model.FileVitals) ListItem {
	return ListItem{
		Path:   r.Path,
		Title:  titleOf(r),
		Status: fmStr(r.Frontmatter, "status"),
		Type:   fmStr(r.Frontmatter, "type"),
		Tags:   fmTags(r.Frontmatter),
	}
}

// recordTags is the deduped union of frontmatter tags and #hashtag markers.
func recordTags(r model.FileVitals) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, t := range fmTags(r.Frontmatter) {
		add(t)
	}
	for _, m := range r.Markers {
		if m.Kind == model.MarkerHashtag {
			add(m.Value)
		}
	}
	return out
}

// TagCount is one entry of the tag index.
type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// Tags returns the tag index (count per tag) under root, sorted by count desc
// then name asc.
func Tags(root string) ([]TagCount, error) {
	recs, _, err := scan.Collect(root)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, r := range recs {
		for _, t := range recordTags(r) {
			counts[t]++
		}
	}
	out := make([]TagCount, 0, len(counts))
	for t, c := range counts {
		out = append(out, TagCount{Tag: t, Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Tag < out[j].Tag
	})
	return out, nil
}

// TagGroup is a tag plus its member records.
type TagGroup struct {
	Tag     string     `json:"tag"`
	Count   int        `json:"count"`
	Records []ListItem `json:"records"`
}

// TagGroups returns groups for tags whose name contains substr (all tags when
// substr is ""), each with its member records. Sorted by count desc, name asc.
func TagGroups(root, substr string) ([]TagGroup, error) {
	recs, _, err := scan.Collect(root)
	if err != nil {
		return nil, err
	}
	ls := strings.ToLower(substr)
	byTag := map[string][]ListItem{}
	for _, r := range recs {
		item := Summarize(r)
		for _, t := range recordTags(r) {
			if substr == "" || strings.Contains(strings.ToLower(t), ls) {
				byTag[t] = append(byTag[t], item)
			}
		}
	}
	out := make([]TagGroup, 0, len(byTag))
	for t, members := range byTag {
		sort.Slice(members, func(i, j int) bool { return members[i].Path < members[j].Path })
		out = append(out, TagGroup{Tag: t, Count: len(members), Records: members})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Tag < out[j].Tag
	})
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
