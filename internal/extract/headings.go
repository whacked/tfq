package extract

import (
	"regexp"
	"strings"

	"tfq/internal/model"
)

var (
	reMdHeading  = regexp.MustCompile(`(?m)^(#{1,6})\s+(.*\S)\s*$`)
	reOrgHeading = regexp.MustCompile(`(?m)^(\*+)\s+(.*\S)\s*$`)
)

// Headings extracts section headings. Org files use leading '*'; everything
// else uses markdown '#'. Level is the count of marker characters. Never fails.
func Headings(content, format string) ([]model.Heading, []model.Warning) {
	re := reMdHeading
	if format == "org" {
		re = reOrgHeading
	}
	out := []model.Heading{}
	for _, m := range re.FindAllStringSubmatchIndex(content, -1) {
		level := m[3] - m[2] // length of the marker capture group
		text := strings.TrimSpace(content[m[4]:m[5]])
		line, _ := lineCol(content, m[0])
		out = append(out, model.Heading{Level: level, Text: text, Line: line})
	}
	return out, nil
}
