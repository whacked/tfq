package validate

import (
	"sort"

	"tfq/internal/cueschema"
	"tfq/internal/engine"
	"tfq/internal/graph"
	"tfq/internal/model"
	"tfq/internal/scan"
)

// Finding is one validation result for one record.
type Finding struct {
	Path     string `json:"path"`
	Field    string `json:"field"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
}

// Report is the full validation result over a directory.
type Report struct {
	Findings []Finding `json:"findings"`
	OK       bool      `json:"ok"`
}

// Run validates every record under root against the discovered .tfq.cue (if any)
// and against graph edge resolution. strict promotes findings to errors.
func Run(root string, strict bool) (Report, error) {
	return RunWith(root, strict, "")
}

// File validates a single file's frontmatter against schemaPath, with cue vet
// semantics: any violation is an error and OK is false. When schemaPath is empty
// the nearest ancestor .tfq.cue is used; if no schema is found the file is
// reported OK with no findings. Graph/edge checks do not apply to a lone file.
func File(filePath, schemaPath string) (Report, error) {
	fv, err := engine.Inspect(filePath)
	if err != nil {
		return Report{}, err
	}
	s, ok, lerr := loadSchema(schemaPath, filePath)
	if lerr != nil {
		return Report{Findings: []Finding{{Path: filePath, Message: "schema load error: " + lerr.Error(), Severity: "error"}}}, nil
	}
	findings := []Finding{}
	if ok {
		for _, v := range s.Validate(fv.Frontmatter) {
			findings = append(findings, Finding{Path: filePath, Field: v.Field, Message: v.Message, Severity: "error"})
		}
	}
	return Report{Findings: findings, OK: len(findings) == 0}, nil
}

// loadSchema resolves the schema to use: the explicit schemaPath when set, else
// the nearest ancestor .tfq.cue walking up from startPath's directory.
func loadSchema(schemaPath, startPath string) (*cueschema.Schema, bool, error) {
	if schemaPath != "" {
		s, err := cueschema.Load(schemaPath)
		return s, err == nil, err
	}
	if path, found := cueschema.Find(startPath); found {
		s, err := cueschema.Load(path)
		return s, err == nil, err
	}
	return nil, false, nil
}

// RunWith is Run with an explicit schema path. When schemaPath is empty the
// discovered .tfq.cue is used (the Run behavior).
func RunWith(root string, strict bool, schemaPath string) (Report, error) {
	sev := "warning"
	if strict {
		sev = "error"
	}
	findings := []Finding{}

	recs, scanWarns, err := scan.Collect(root)
	if err != nil {
		return Report{}, err
	}
	for _, w := range scanWarns {
		findings = append(findings, Finding{Path: "", Field: "", Message: w.Message, Severity: "warning"})
	}

	// schema: explicit override, else discovered (optional)
	opts := graph.DefaultOptions()
	schemaToLoad, haveSchema := schemaPath, schemaPath != ""
	if !haveSchema {
		schemaToLoad, haveSchema = cueschema.Find(root)
	}
	if haveSchema {
		if s, lerr := cueschema.Load(schemaToLoad); lerr == nil {
			for _, r := range recs {
				for _, v := range s.Validate(r.Frontmatter) {
					findings = append(findings, Finding{Path: r.Path, Field: v.Field, Message: v.Message, Severity: sev})
				}
			}
			if efs := s.EdgeFields(); len(efs) > 0 {
				names := make([]string, len(efs))
				for i, e := range efs {
					names[i] = e.Name
				}
				opts = graph.Options{FrontmatterEdgeFields: names}
			}
		} else {
			findings = append(findings, Finding{Message: "schema load error: " + lerr.Error(), Severity: sev})
		}
	}

	// graph dangling edges
	g := graph.Build(recs, opts)
	for _, w := range g.Warnings() {
		findings = append(findings, Finding{Path: pathOf(w), Field: "", Message: w.Message, Severity: sev})
	}

	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Path != findings[j].Path {
			return findings[i].Path < findings[j].Path
		}
		return findings[i].Message < findings[j].Message
	})

	ok := true
	for _, f := range findings {
		if f.Severity == "error" {
			ok = false
		}
	}
	return Report{Findings: findings, OK: ok}, nil
}

// pathOf extracts the leading "path:" prefix from a graph warning message.
func pathOf(w model.Warning) string {
	msg := w.Message
	for i := 0; i < len(msg); i++ {
		if msg[i] == ':' {
			return msg[:i]
		}
	}
	return ""
}
