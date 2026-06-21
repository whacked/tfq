package extract

import "strings"

// lineCol converts a byte offset into a 1-based (line, col).
// col is the byte position within the line, also 1-based.
func lineCol(content string, byteOffset int) (int, int) {
	if byteOffset < 0 {
		byteOffset = 0
	}
	if byteOffset > len(content) {
		byteOffset = len(content)
	}
	prefix := content[:byteOffset]
	line := strings.Count(prefix, "\n") + 1
	col := byteOffset - (strings.LastIndex(prefix, "\n") + 1) + 1
	return line, col
}
