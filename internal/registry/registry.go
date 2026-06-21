package registry

import "strings"

// FormatFor maps a file extension (with or without leading dot, any case) to
// a format name. Unknown extensions fall back to "text".
func FormatFor(ext string) string {
	e := strings.ToLower(strings.TrimPrefix(ext, "."))
	switch e {
	case "md", "markdown":
		return "markdown"
	case "org":
		return "org"
	default:
		return "text"
	}
}
