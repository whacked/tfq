package cueschema

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
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
