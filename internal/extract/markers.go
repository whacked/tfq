package extract

import (
	"regexp"
	"strings"

	"tfq/internal/model"
)

var (
	reHashtag     = regexp.MustCompile(`(^|[^\w&])#([A-Za-z0-9][\w/-]*)`)
	reDoubleAngle = regexp.MustCompile(`<<([^<>]+)>>`)
	reSingleAngle = regexp.MustCompile(`<([^<>]+)>`)
	reOrgTagLine  = regexp.MustCompile(`(?m)(\s):([A-Za-z0-9_@%][A-Za-z0-9_@%:]*):\s*$`)
	reURLish      = regexp.MustCompile(`^(?:[a-zA-Z][a-zA-Z0-9+.-]*://|mailto:)`)
)

// Markers extracts hashtags, angle-bracket phrases, and (org only) org tags.
// Never fails.
func Markers(content, format string) ([]model.Marker, []model.Warning) {
	out := []model.Marker{}
	consumed := map[int]bool{} // byte offsets covered by an accepted << >> marker

	// hashtags
	for _, m := range reHashtag.FindAllStringSubmatchIndex(content, -1) {
		start, end := m[4], m[5] // group 2 (without leading boundary / '#')
		line, col := lineCol(content, start-1) // point at the '#'
		out = append(out, model.Marker{Kind: model.MarkerHashtag, Value: content[start:end], Line: line, Col: col})
	}

	// double-angle first (claims its byte range)
	for _, m := range reDoubleAngle.FindAllStringSubmatchIndex(content, -1) {
		for i := m[0]; i < m[1]; i++ {
			consumed[i] = true
		}
		line, col := lineCol(content, m[0])
		out = append(out, model.Marker{Kind: model.MarkerDoubleAngle, Value: content[m[2]:m[3]], Line: line, Col: col})
	}

	// single-angle, skipping ranges already consumed by << >> and url-ish values
	for _, m := range reSingleAngle.FindAllStringSubmatchIndex(content, -1) {
		if consumed[m[0]] || consumed[m[1]-1] {
			continue
		}
		val := content[m[2]:m[3]]
		if reURLish.MatchString(val) {
			continue
		}
		line, col := lineCol(content, m[0])
		out = append(out, model.Marker{Kind: model.MarkerAngle, Value: val, Line: line, Col: col})
	}

	// org tags (org format only)
	if format == "org" {
		for _, m := range reOrgTagLine.FindAllStringSubmatchIndex(content, -1) {
			group := content[m[4]:m[5]] // e.g. "work:urgent"
			base := m[4]
			offset := 0
			for _, tag := range strings.Split(group, ":") {
				if tag == "" {
					offset++
					continue
				}
				line, col := lineCol(content, base+offset)
				out = append(out, model.Marker{Kind: model.MarkerOrgTag, Value: tag, Line: line, Col: col})
				offset += len(tag) + 1
			}
		}
	}

	return out, nil
}
