package layout

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Template selects a path rule.
type Template string

const (
	TemplateNote Template = "note"
	TemplateTask Template = "task"
)

// Rule is the path policy for one template.
type Rule struct {
	Dir      string // pattern, e.g. "{yyyy}/{mm}"
	File     string // pattern, e.g. "{yyyy}-{mm}-{dd}.{nnn}-{slug}.md"
	Sequence string // "daily" | "global"
	Padding  int
}

// Config is the full path policy. Plain struct so it can be loaded from
// user config in the future; DefaultConfig encodes today's conventions.
type Config struct {
	Rules map[Template]Rule
}

// DefaultConfig replicates the agent-resources note/task conventions.
func DefaultConfig() Config {
	return Config{Rules: map[Template]Rule{
		TemplateNote: {Dir: "{yyyy}/{mm}", File: "{yyyy}-{mm}-{dd}.{nnn}-{slug}.md", Sequence: "daily", Padding: 3},
		TemplateTask: {Dir: "{yyyy}/{mm}", File: "{nnn}-{slug}.md", Sequence: "global", Padding: 3},
	}}
}

func (c Config) rule(tmpl Template) (Rule, error) {
	r, ok := c.Rules[tmpl]
	if !ok {
		return Rule{}, fmt.Errorf("unknown template %q", tmpl)
	}
	return r, nil
}

func subst(pattern string, date time.Time, seq, padding int, slug string) string {
	rep := strings.NewReplacer(
		"{yyyy}", date.Format("2006"),
		"{mm}", date.Format("01"),
		"{dd}", date.Format("02"),
		"{nnn}", fmt.Sprintf("%0*d", padding, seq),
		"{slug}", slug,
	)
	return rep.Replace(pattern)
}

// RelPath computes the collection-relative path for a new record.
func (c Config) RelPath(tmpl Template, slug string, date time.Time, seq int) (string, error) {
	r, err := c.rule(tmpl)
	if err != nil {
		return "", err
	}
	dir := subst(r.Dir, date, seq, r.Padding, slug)
	file := subst(r.File, date, seq, r.Padding, slug)
	return filepath.ToSlash(filepath.Join(dir, file)), nil
}

var leadingInt = regexp.MustCompile(`^(\d+)-`)

// NextSequence computes the next sequence number under root for the template.
func (c Config) NextSequence(root string, tmpl Template, date time.Time) (int, error) {
	r, err := c.rule(tmpl)
	if err != nil {
		return 0, err
	}
	max := 0
	if r.Sequence == "daily" {
		shard := filepath.Join(root, subst(r.Dir, date, 0, r.Padding, ""))
		prefix := date.Format("2006-01-02") + "."
		entries, derr := filepath.Glob(filepath.Join(shard, prefix+"*"))
		if derr != nil {
			return 0, derr
		}
		re := regexp.MustCompile(`\.(\d+)-`)
		for _, p := range entries {
			if m := re.FindStringSubmatch(filepath.Base(p)); m != nil {
				if n, _ := strconv.Atoi(m[1]); n > max {
					max = n
				}
			}
		}
		return max + 1, nil
	}
	// global: max leading integer among *.md basenames under root
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		if m := leadingInt.FindStringSubmatch(d.Name()); m != nil {
			if n, _ := strconv.Atoi(m[1]); n > max {
				max = n
			}
		}
		return nil
	})
	return max + 1, nil
}
