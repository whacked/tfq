package cueschema

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

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

// Load compiles a .tfq.cue file.
func Load(path string) (*Schema, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	ctx := cuecontext.New()
	v := ctx.CompileBytes(b)
	if v.Err() != nil {
		return nil, v.Err()
	}
	return &Schema{ctx: ctx, value: v}, nil
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
	data := s.ctx.Encode(fm)
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
