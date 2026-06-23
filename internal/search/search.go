package search

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"tfq/internal/engine"
	"tfq/internal/model"
)

// Hit is a single ripgrep match, classified by the structure it lands in.
type Hit struct {
	Path  string   `json:"path"`
	Line  int      `json:"line"`
	Text  string   `json:"text"`
	Kinds []string `json:"kinds,omitempty"` // any of heading|tag|link
}

// Filters narrows hits by frontmatter (AND semantics; empty matches all). In
// restricts hits to those landing in the named structures (heading|tag|link).
type Filters struct {
	Type       string
	Status     string
	Tags       []string
	In         []string
	IgnoreCase bool
}

type rgEvent struct {
	Type string `json:"type"`
	Data struct {
		Path struct {
			Text string `json:"text"`
		} `json:"path"`
		LineNumber int `json:"line_number"`
		Lines      struct {
			Text string `json:"text"`
		} `json:"lines"`
	} `json:"data"`
}

// Search runs ripgrep over root and post-filters by frontmatter.
func Search(root, query string, f Filters) ([]Hit, []model.Warning, error) {
	rgArgs := []string{"--json", "--line-number"}
	if f.IgnoreCase {
		rgArgs = append(rgArgs, "-i")
	}
	rgArgs = append(rgArgs, "--", query, root)
	cmd := exec.Command("rg", rgArgs...)
	out, err := cmd.Output()
	if err != nil {
		// rg exits 1 when there are no matches; that is not an error.
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return []Hit{}, nil, nil
		}
		return nil, nil, err
	}

	var warns []model.Warning
	cache := map[string]model.FileVitals{}
	inspect := func(abs string) (model.FileVitals, bool) {
		if v, ok := cache[abs]; ok {
			return v, true
		}
		b, rerr := os.ReadFile(abs)
		if rerr != nil {
			warns = append(warns, model.Warning{Module: "search", Message: "cannot read " + abs})
			return model.FileVitals{}, false
		}
		v := engine.InspectContent(abs, string(b))
		cache[abs] = v
		return v, true
	}

	var qre *regexp.Regexp
	pat := query
	if f.IgnoreCase {
		pat = "(?i)" + pat
	}
	if r, cerr := regexp.Compile(pat); cerr == nil {
		qre = r
	}
	hasFilters := f.Type != "" || f.Status != "" || len(f.Tags) > 0

	hits := []Hit{}
	for _, line := range strings.Split(string(out), "\n") {
		if line == "" {
			continue
		}
		var ev rgEvent
		if json.Unmarshal([]byte(line), &ev) != nil || ev.Type != "match" {
			continue
		}
		abs := ev.Data.Path.Text
		v, ok := inspect(abs)
		if hasFilters && (!ok || !passesFilters(v, f)) {
			continue
		}
		var kinds []string
		if ok {
			kinds = classify(v, ev.Data.LineNumber, qre, query, f.IgnoreCase)
		}
		if len(f.In) > 0 && !anyIn(kinds, f.In) {
			continue
		}
		rel, rerr := filepath.Rel(root, abs)
		if rerr != nil {
			rel = abs
		}
		hits = append(hits, Hit{
			Path:  filepath.ToSlash(rel),
			Line:  ev.Data.LineNumber,
			Text:  strings.TrimRight(ev.Data.Lines.Text, "\n"),
			Kinds: kinds,
		})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Path != hits[j].Path {
			return hits[i].Path < hits[j].Path
		}
		return hits[i].Line < hits[j].Line
	})
	return hits, warns, nil
}

func passesFilters(v model.FileVitals, f Filters) bool {
	if f.Type != "" {
		t, ok := v.Frontmatter["type"].(string)
		if !ok || t != f.Type {
			return false
		}
	}
	if f.Status != "" {
		s, ok := v.Frontmatter["status"].(string)
		if !ok || s != f.Status {
			return false
		}
	}
	for _, tag := range f.Tags {
		if !hasTag(v, tag) {
			return false
		}
	}
	return true
}

// classify labels a hit by the structural elements on its line whose value
// matches the query (headings match by line alone).
func classify(v model.FileVitals, line int, q *regexp.Regexp, raw string, ic bool) []string {
	kinds := []string{}
	for _, h := range v.Headings {
		if h.Line == line {
			kinds = append(kinds, "heading")
			break
		}
	}
	for _, m := range v.Markers {
		if (m.Kind == model.MarkerHashtag || m.Kind == model.MarkerOrgTag) && m.Line == line && matchVal(m.Value, q, raw, ic) {
			kinds = append(kinds, "tag")
			break
		}
	}
	for _, l := range v.Links {
		label := ""
		if l.Label != nil {
			label = *l.Label
		}
		if l.Line == line && (matchVal(l.Target, q, raw, ic) || matchVal(label, q, raw, ic)) {
			kinds = append(kinds, "link")
			break
		}
	}
	return kinds
}

// matchVal reports whether val matches the query (regexp when it compiled,
// else case-aware substring).
func matchVal(val string, q *regexp.Regexp, raw string, ic bool) bool {
	if val == "" {
		return false
	}
	if q != nil {
		return q.MatchString(val)
	}
	if ic {
		return strings.Contains(strings.ToLower(val), strings.ToLower(raw))
	}
	return strings.Contains(val, raw)
}

// anyIn reports whether any element of have is in want.
func anyIn(have, want []string) bool {
	for _, h := range have {
		for _, w := range want {
			if h == w {
				return true
			}
		}
	}
	return false
}

func hasTag(v model.FileVitals, tag string) bool {
	for _, m := range v.Markers {
		if m.Kind == model.MarkerHashtag && m.Value == tag {
			return true
		}
	}
	switch t := v.Frontmatter["tags"].(type) {
	case []any:
		for _, e := range t {
			if s, ok := e.(string); ok && s == tag {
				return true
			}
		}
	case []string:
		for _, s := range t {
			if s == tag {
				return true
			}
		}
	}
	return false
}
