package store

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"tfq/internal/layout"
)

// WriteResult reports a create/update.
type WriteResult struct {
	Path   string `json:"path"`
	Action string `json:"action"`
}

var slugRe = regexp.MustCompile(`^[a-z0-9-]+$`)

// New creates a record file under root using the layout config and a template.
func New(root string, tmpl layout.Template, slug string, fields map[string]string, date time.Time, cfg layout.Config) (WriteResult, error) {
	if !slugRe.MatchString(slug) {
		return WriteResult{}, fmt.Errorf("slug %q must match [a-z0-9-]+", slug)
	}
	seq, err := cfg.NextSequence(root, tmpl, date)
	if err != nil {
		return WriteResult{}, err
	}
	rel, err := cfg.RelPath(tmpl, slug, date, seq)
	if err != nil {
		return WriteResult{}, err
	}
	full := filepath.Join(root, rel)
	if _, err := os.Stat(full); err == nil {
		return WriteResult{}, fmt.Errorf("file already exists: %s", rel)
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return WriteResult{}, err
	}
	content := scaffold(tmpl, slug, date, seq, cfg, fields)
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		return WriteResult{}, err
	}
	return WriteResult{Path: rel, Action: "created"}, nil
}

func titleWords(slug string) string {
	return strings.ReplaceAll(slug, "-", " ")
}

// scaffold builds frontmatter + body for a new record. fields override defaults.
func scaffold(tmpl layout.Template, slug string, date time.Time, seq int, cfg layout.Config, fields map[string]string) string {
	pad := cfg.Rules[tmpl].Padding
	var fm []string
	var body string
	switch tmpl {
	case layout.TemplateTask:
		base := map[string]string{
			"id":       fmt.Sprintf("%0*d", pad, seq),
			"title":    titleWords(slug),
			"status":   "pending",
			"priority": "medium",
		}
		order := []string{"id", "title", "status", "priority"}
		fm = renderFM(base, order, fields)
		body = "# " + titleWords(slug) + "\n"
	default: // note
		base := map[string]string{
			"date":   date.Format("2006-01-02"),
			"author": "agent",
			"slug":   slug,
		}
		order := []string{"date", "author", "slug"}
		fm = renderFM(base, order, fields)
		fm = append(fm, "source_notes: []", "tags: []")
		body = "# " + titleWords(slug) + "\n\n<summary>\n"
	}
	return "---\n" + strings.Join(fm, "\n") + "\n---\n" + body
}

// renderFM emits "key: value" lines for base keys in order, with fields
// overriding values and any extra fields appended in sorted order.
func renderFM(base map[string]string, order []string, fields map[string]string) []string {
	out := []string{}
	used := map[string]bool{}
	for _, k := range order {
		v := base[k]
		if fields != nil {
			if ov, ok := fields[k]; ok {
				v = ov
			}
		}
		used[k] = true
		out = append(out, k+": "+v)
	}
	extra := []string{}
	for k := range fields {
		if !used[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	for _, k := range extra {
		out = append(out, k+": "+fields[k])
	}
	return out
}
