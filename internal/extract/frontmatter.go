package extract

import (
	"strings"

	"gopkg.in/yaml.v3"
	"tfq/internal/model"
)

// Frontmatter extracts a leading YAML frontmatter block delimited by lines
// containing only "---". It returns the parsed map, the body with the
// frontmatter region blanked (line count preserved), and any warnings.
func Frontmatter(content string) (map[string]any, string, []model.Warning) {
	empty := map[string]any{}
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r") != "---" {
		return empty, content, nil
	}
	// find the closing fence
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r") == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		// no closing fence: treat as no frontmatter (liberal)
		return empty, content, []model.Warning{{Module: "frontmatter", Message: "opening --- without closing ---; ignored"}}
	}
	yamlSrc := strings.Join(lines[1:end], "\n")

	var fm map[string]any
	var warns []model.Warning
	if err := yaml.Unmarshal([]byte(yamlSrc), &fm); err != nil || fm == nil {
		if err != nil {
			warns = append(warns, model.Warning{Module: "frontmatter", Message: "yaml parse error: " + err.Error()})
		}
		fm = map[string]any{}
	}

	// blank lines 0..end inclusive to preserve absolute line numbers downstream
	blanked := make([]string, len(lines))
	for i := range lines {
		if i <= end {
			blanked[i] = ""
		} else {
			blanked[i] = lines[i]
		}
	}
	return fm, strings.Join(blanked, "\n"), warns
}
