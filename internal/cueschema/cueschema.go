package cueschema

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	cueerrors "cuelang.org/go/cue/errors"
)

// EdgeField names a frontmatter field declared as a graph edge via @edge.
type EdgeField struct {
	Name     string
	Blocking bool
}

// Schema is a compiled .tfq.cue collection schema.
type Schema struct {
	ctx   *cue.Context
	value cue.Value
}

// Load compiles a CUE schema file. The file may be raw CUE (e.g. .tfq.cue) or a
// markdown document whose first ```cue fenced block holds the schema (the
// agent-resources *.cue.template.md convention) — in that case only the block
// is compiled.
func Load(path string) (*Schema, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if block, ok := extractCueBlock(b); ok {
		b = block
	}
	ctx := cuecontext.New()
	v := ctx.CompileBytes(b)
	if v.Err() != nil {
		return nil, v.Err()
	}
	return &Schema{ctx: ctx, value: v}, nil
}

// extractCueBlock returns the contents of the first ```cue ... ``` fenced block,
// or (nil, false) if the source has no such fence (treat as raw CUE).
func extractCueBlock(src []byte) ([]byte, bool) {
	lines := strings.Split(string(src), "\n")
	start := -1
	for i, ln := range lines {
		if strings.TrimSpace(ln) == "```cue" {
			start = i + 1
			break
		}
	}
	if start == -1 {
		return nil, false
	}
	for i := start; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "```" {
			return []byte(strings.Join(lines[start:i], "\n")), true
		}
	}
	return nil, false
}

// Find walks up from startDir returning the first .tfq.cue path found.
func Find(startDir string) (string, bool) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		dir = startDir
	}
	for {
		cand := filepath.Join(dir, ".tfq.cue")
		if _, err := os.Stat(cand); err == nil {
			return cand, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// Violation is a single schema rule that the frontmatter failed.
type Violation struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Validate checks a frontmatter map against the schema. Returns an empty slice
// when valid, or one Violation per failed rule.
func (s *Schema) Validate(fm map[string]any) []Violation {
	out := []Violation{}
	data := s.ctx.Encode(normalizeTimes(fm))
	if data.Err() != nil {
		return []Violation{{Field: "", Message: "cannot encode frontmatter: " + data.Err().Error()}}
	}
	unified := s.value.Unify(data)
	if err := unified.Validate(cue.Concrete(true), cue.All()); err != nil {
		for _, e := range cueerrors.Errors(err) {
			field := ""
			if p := e.Path(); len(p) > 0 {
				field = strings.Join(p, ".")
			}
			out = append(out, Violation{Field: field, Message: e.Error()})
		}
		if len(out) == 0 {
			out = append(out, Violation{Field: "", Message: err.Error()})
		}
	}
	return out
}

// normalizeTimes deep-copies v, converting any time.Time to the string form a
// YAML author would have written: "2006-01-02" for a midnight-UTC value (a bare
// date), else RFC3339. yaml.v3 parses unquoted timestamps into time.Time, which
// ctx.Encode would otherwise render as RFC3339 — diverging from how `cue vet`
// (whose YAML decoder keeps timestamps as strings) sees the same frontmatter.
func normalizeTimes(v any) any {
	switch t := v.(type) {
	case time.Time:
		if t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 && t.Nanosecond() == 0 && t.Location() == time.UTC {
			return t.Format("2006-01-02")
		}
		return t.Format(time.RFC3339)
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = normalizeTimes(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = normalizeTimes(val)
		}
		return out
	default:
		return v
	}
}

// EdgeFields returns the frontmatter fields marked with @edge, sorted by name.
func (s *Schema) EdgeFields() []EdgeField {
	out := []EdgeField{}
	iter, err := s.value.Fields(cue.Optional(true))
	if err != nil {
		return out
	}
	for iter.Next() {
		name := strings.TrimSuffix(iter.Selector().String(), "?")
		attr := iter.Value().Attribute("edge")
		if attr.Err() != nil {
			continue
		}
		blocking := false
		if attr.NumArgs() > 0 {
			if arg, _ := attr.String(0); arg == "blocking" {
				blocking = true
			}
		}
		out = append(out, EdgeField{Name: name, Blocking: blocking})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
