package main

import (
	"fmt"
	"sort"
	"strings"

	"tfq/internal/graph"
	"tfq/internal/model"
	"tfq/internal/query"
	"tfq/internal/search"
	"tfq/internal/store"
	"tfq/internal/validate"
)

// summaryLine renders "path  <type> <status> #tag…" (omitting empty parts).
func summaryLine(path, typ, status string, tags []string) string {
	meta := []string{}
	if typ != "" {
		meta = append(meta, typ)
	}
	if status != "" {
		meta = append(meta, status)
	}
	for _, t := range tags {
		meta = append(meta, "#"+t)
	}
	if len(meta) == 0 {
		return path
	}
	return path + "  " + strings.Join(meta, " ")
}

func formatList(items []query.ListItem) string {
	blocks := make([]string, 0, len(items))
	for _, it := range items {
		b := summaryLine(it.Path, it.Type, it.Status, it.Tags)
		if it.Title != "" {
			b += "\n  title: " + it.Title
		}
		blocks = append(blocks, b)
	}
	return strings.Join(blocks, "\n\n")
}

func formatHits(hits []search.Hit, heading bool) string {
	if len(hits) == 0 {
		return ""
	}
	if !heading {
		lines := make([]string, len(hits))
		for i, h := range hits {
			lines[i] = fmt.Sprintf("%s:%d:%s", h.Path, h.Line, h.Text)
		}
		return strings.Join(lines, "\n")
	}
	var blocks []string
	cur := ""
	var curLines []string
	flush := func() {
		if cur != "" {
			blocks = append(blocks, cur+"\n"+strings.Join(curLines, "\n"))
		}
	}
	for _, h := range hits {
		if h.Path != cur {
			flush()
			cur, curLines = h.Path, nil
		}
		curLines = append(curLines, fmt.Sprintf("%d: %s", h.Line, h.Text))
	}
	flush()
	return strings.Join(blocks, "\n\n")
}

func filesOf(hits []search.Hit) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, h := range hits {
		if !seen[h.Path] {
			seen[h.Path] = true
			out = append(out, h.Path)
		}
	}
	return out
}

type fileCount struct {
	Path  string `json:"path"`
	Count int    `json:"count"`
}

func countsOf(hits []search.Hit) []fileCount {
	order := []string{}
	m := map[string]int{}
	for _, h := range hits {
		if _, ok := m[h.Path]; !ok {
			order = append(order, h.Path)
		}
		m[h.Path]++
	}
	out := make([]fileCount, len(order))
	for i, p := range order {
		out[i] = fileCount{Path: p, Count: m[p]}
	}
	return out
}

func formatCounts(counts []fileCount) string {
	lines := make([]string, len(counts))
	for i, c := range counts {
		lines[i] = fmt.Sprintf("%s:%d", c.Path, c.Count)
	}
	return strings.Join(lines, "\n")
}

func formatTagsIndex(tags []query.TagCount) string {
	if len(tags) == 0 {
		return ""
	}
	w := 0
	for _, t := range tags {
		if len(t.Tag) > w {
			w = len(t.Tag)
		}
	}
	lines := make([]string, len(tags))
	for i, t := range tags {
		lines[i] = fmt.Sprintf("  %-*s  %d", w, t.Tag, t.Count)
	}
	return "# tags\n" + strings.Join(lines, "\n")
}

func formatTagGroups(groups []query.TagGroup) string {
	blocks := make([]string, 0, len(groups))
	for _, g := range groups {
		var b strings.Builder
		fmt.Fprintf(&b, "# %s  %d", g.Tag, g.Count)
		for _, r := range g.Records {
			b.WriteString("\n  ==> " + summaryLine(r.Path, r.Type, r.Status, nil))
		}
		blocks = append(blocks, b.String())
	}
	return strings.Join(blocks, "\n\n")
}

func formatLinks(path string, out []graph.Edge, in []string, showOut, showIn bool) string {
	var b strings.Builder
	b.WriteString(path)
	if showOut {
		b.WriteString("\n\n# outbound links")
		if len(out) == 0 {
			b.WriteString("\n  (none)")
		}
		for _, e := range out {
			to := e.To
			if to == "" {
				to = e.Raw + " (unresolved)"
			}
			b.WriteString("\n  ==> " + to + "\n      " + e.Kind + " " + e.Raw)
		}
	}
	if showIn {
		b.WriteString("\n\n# inbound links")
		if len(in) == 0 {
			b.WriteString("\n  (none)")
		}
		for _, p := range in {
			b.WriteString("\n  <== " + p)
		}
	}
	return b.String()
}

func linkedPaths(out []graph.Edge, in []string, showOut, showIn bool) []string {
	seen := map[string]bool{}
	res := []string{}
	add := func(p string) {
		if p != "" && !seen[p] {
			seen[p] = true
			res = append(res, p)
		}
	}
	if showOut {
		for _, e := range out {
			add(e.To)
		}
	}
	if showIn {
		for _, p := range in {
			add(p)
		}
	}
	sort.Strings(res)
	return res
}

func formatRecord(rec query.Record) string {
	out := rec.Path + "\n" + formatFrontmatterBlock(rec.Frontmatter) + "\n\n" + rec.Body
	return strings.TrimRight(out, "\n")
}

func formatFrontmatterBlock(fm map[string]any) string {
	keys := make([]string, 0, len(fm))
	for k := range fm {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, k := range keys {
		lines = append(lines, k+": "+fmScalar(fm[k]))
	}
	return strings.Join(lines, "\n")
}

func fmScalar(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		parts := make([]string, len(t))
		for i, e := range t {
			parts[i] = fmt.Sprintf("%v", e)
		}
		return strings.Join(parts, ", ")
	default:
		return fmt.Sprintf("%v", t)
	}
}

func formatWrite(res store.WriteResult) string {
	return res.Action + " " + res.Path
}

func formatReport(rep validate.Report) string {
	var b strings.Builder
	if rep.OK {
		b.WriteString("validate: OK")
	} else {
		b.WriteString("validate: FAILED")
	}
	for _, f := range rep.Findings {
		loc := f.Path
		if f.Field != "" {
			loc += " [" + f.Field + "]"
		}
		b.WriteString(fmt.Sprintf("\n  %s: %s (%s)", loc, f.Message, f.Severity))
	}
	return b.String()
}

func formatEdges(edges []graph.Edge) string {
	if len(edges) == 0 {
		return ""
	}
	lines := make([]string, len(edges))
	for i, e := range edges {
		to := e.To
		if to == "" {
			to = e.Raw + " (unresolved)"
		}
		lines[i] = fmt.Sprintf("%s --%s--> %s", e.From, e.Kind, to)
	}
	return strings.Join(lines, "\n")
}

func formatInspect(fv model.FileVitals) string {
	return fmt.Sprintf("%s\n  format: %s\n  headings: %d\n  links: %d\n  markers: %d",
		fv.Path, fv.Format, len(fv.Headings), len(fv.Links), len(fv.Markers))
}
