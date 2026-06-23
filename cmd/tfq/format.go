package main

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"tfq/internal/graph"
	"tfq/internal/model"
	"tfq/internal/query"
	"tfq/internal/search"
	"tfq/internal/store"
	"tfq/internal/validate"
)

// summaryLine renders "path  <type> <status> #tag…" (omitting empty parts).
func summaryLine(path, typ, status string, tags []string, p palette) string {
	out := p.path(path)
	meta := []string{}
	if typ != "" {
		meta = append(meta, p.dim(typ))
	}
	if status != "" {
		meta = append(meta, p.statusColor(status))
	}
	for _, t := range tags {
		meta = append(meta, p.tag("#"+t))
	}
	if len(meta) == 0 {
		return out
	}
	return out + "  " + strings.Join(meta, " ")
}

func formatList(items []query.ListItem, p palette) string {
	blocks := make([]string, 0, len(items))
	for _, it := range items {
		b := summaryLine(it.Path, it.Type, it.Status, it.Tags, p)
		if it.Title != "" {
			b += "\n  title: " + it.Title
		}
		blocks = append(blocks, b)
	}
	return strings.Join(blocks, "\n\n")
}

func formatHits(hits []search.Hit, heading bool, m *regexp.Regexp, p palette) string {
	if len(hits) == 0 {
		return ""
	}
	if !heading {
		lines := make([]string, len(hits))
		for i, h := range hits {
			lines[i] = fmt.Sprintf("%s:%s:%s", p.path(h.Path), p.lineNo(strconv.Itoa(h.Line)), highlight(h.Text, m, p))
		}
		return strings.Join(lines, "\n")
	}
	var blocks []string
	cur := ""
	var curLines []string
	flush := func() {
		if cur != "" {
			blocks = append(blocks, p.path(cur)+"\n"+strings.Join(curLines, "\n"))
		}
	}
	for _, h := range hits {
		if h.Path != cur {
			flush()
			cur, curLines = h.Path, nil
		}
		curLines = append(curLines, fmt.Sprintf("%s: %s", p.lineNo(strconv.Itoa(h.Line)), highlight(h.Text, m, p)))
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

// formatPaths renders one colored path per line (used by -l in search/links).
func formatPaths(paths []string, p palette) string {
	out := make([]string, len(paths))
	for i, s := range paths {
		out[i] = p.path(s)
	}
	return strings.Join(out, "\n")
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
	for i, pth := range order {
		out[i] = fileCount{Path: pth, Count: m[pth]}
	}
	return out
}

func formatCounts(counts []fileCount, p palette) string {
	lines := make([]string, len(counts))
	for i, c := range counts {
		lines[i] = fmt.Sprintf("%s:%d", p.path(c.Path), c.Count)
	}
	return strings.Join(lines, "\n")
}

func formatTagsIndex(tags []query.TagCount, p palette) string {
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
		pad := strings.Repeat(" ", w-len(t.Tag))
		lines[i] = fmt.Sprintf("  %s%s  %d", p.tag(t.Tag), pad, t.Count)
	}
	return p.bold("# tags") + "\n" + strings.Join(lines, "\n")
}

func formatTagGroups(groups []query.TagGroup, p palette) string {
	blocks := make([]string, 0, len(groups))
	for _, g := range groups {
		var b strings.Builder
		b.WriteString(p.bold("# "+g.Tag) + fmt.Sprintf("  %d", g.Count))
		for _, r := range g.Records {
			b.WriteString("\n  " + p.dim("==>") + " " + summaryLine(r.Path, r.Type, r.Status, nil, p))
		}
		blocks = append(blocks, b.String())
	}
	return strings.Join(blocks, "\n\n")
}

func formatLinks(path string, out []graph.Edge, in []string, showOut, showIn bool, p palette) string {
	var b strings.Builder
	b.WriteString(p.path(path))
	if showOut {
		b.WriteString("\n\n" + p.bold("# outbound links"))
		if len(out) == 0 {
			b.WriteString("\n  (none)")
		}
		for _, e := range out {
			to := p.path(e.To)
			if e.To == "" {
				to = p.dim(e.Raw + " (unresolved)")
			}
			b.WriteString("\n  " + p.dim("==>") + " " + to)
			b.WriteString("\n      " + p.dim(e.Kind+" "+e.Raw))
		}
	}
	if showIn {
		b.WriteString("\n\n" + p.bold("# inbound links"))
		if len(in) == 0 {
			b.WriteString("\n  (none)")
		}
		for _, src := range in {
			b.WriteString("\n  " + p.dim("<==") + " " + p.path(src))
		}
	}
	return b.String()
}

func linkedPaths(out []graph.Edge, in []string, showOut, showIn bool) []string {
	seen := map[string]bool{}
	res := []string{}
	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			res = append(res, s)
		}
	}
	if showOut {
		for _, e := range out {
			add(e.To)
		}
	}
	if showIn {
		for _, s := range in {
			add(s)
		}
	}
	sort.Strings(res)
	return res
}

func formatRecord(rec query.Record, p palette) string {
	out := p.path(rec.Path) + "\n" + formatFrontmatterBlock(rec.Frontmatter, p) + "\n\n" + rec.Body
	return strings.TrimRight(out, "\n")
}

func formatFrontmatterBlock(fm map[string]any, p palette) string {
	keys := make([]string, 0, len(fm))
	for k := range fm {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, k := range keys {
		lines = append(lines, p.dim(k+":")+" "+fmScalar(fm[k]))
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

func formatWrite(res store.WriteResult, p palette) string {
	return res.Action + " " + p.path(res.Path)
}

func formatReport(rep validate.Report, p palette) string {
	var b strings.Builder
	if rep.OK {
		b.WriteString(p.wrap("32", "validate: OK"))
	} else {
		b.WriteString(p.wrap("31", "validate: FAILED"))
	}
	for _, f := range rep.Findings {
		loc := f.Path
		if f.Field != "" {
			loc += " [" + f.Field + "]"
		}
		b.WriteString(fmt.Sprintf("\n  %s: %s (%s)", loc, f.Message, p.severity(f.Severity)))
	}
	return b.String()
}

func formatEdges(edges []graph.Edge, p palette) string {
	if len(edges) == 0 {
		return ""
	}
	lines := make([]string, len(edges))
	for i, e := range edges {
		to := p.path(e.To)
		if e.To == "" {
			to = p.dim(e.Raw + " (unresolved)")
		}
		lines[i] = fmt.Sprintf("%s %s %s", p.path(e.From), p.dim("--"+e.Kind+"-->"), to)
	}
	return strings.Join(lines, "\n")
}

func formatInspect(fv model.FileVitals, p palette) string {
	return fmt.Sprintf("%s\n  format: %s\n  headings: %d\n  links: %d\n  markers: %d",
		p.path(fv.Path), fv.Format, len(fv.Headings), len(fv.Links), len(fv.Markers))
}
