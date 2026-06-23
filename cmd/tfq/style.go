package main

import "regexp"

// palette wraps text in ANSI SGR codes when on. Off, every method is identity,
// so the same formatters produce plain text for pipes/tests and color for TTYs.
type palette struct{ on bool }

func (p palette) wrap(code, s string) string {
	if !p.on || s == "" {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func (p palette) path(s string) string   { return p.wrap("35;1", s) } // magenta bold
func (p palette) lineNo(s string) string { return p.wrap("32", s) }   // green
func (p palette) match(s string) string  { return p.wrap("1;31", s) } // bold red
func (p palette) tag(s string) string    { return p.wrap("36", s) }   // cyan
func (p palette) bold(s string) string   { return p.wrap("1", s) }
func (p palette) dim(s string) string    { return p.wrap("2", s) }

// statusColor tints common task statuses; unknown statuses stay plain.
func (p palette) statusColor(s string) string {
	switch s {
	case "done", "completed", "cancelled":
		return p.wrap("32", s) // green
	case "pending", "blocked", "in-progress":
		return p.wrap("33", s) // yellow
	}
	return s
}

// severity tints validation finding severities.
func (p palette) severity(s string) string {
	switch s {
	case "error":
		return p.wrap("31", s) // red
	case "warning":
		return p.wrap("33", s) // yellow
	}
	return s
}

// decideColor implements ripgrep-like policy: always/never force the outcome;
// auto (the default) enables color only on a TTY when NO_COLOR is unset.
func decideColor(mode string, noColor, isTTY bool) bool {
	switch mode {
	case "always":
		return true
	case "never":
		return false
	default: // auto / unset
		return isTTY && !noColor
	}
}

// highlight wraps each regexp match in line with the match style. A nil matcher
// or a disabled palette leaves the line untouched.
func highlight(line string, m *regexp.Regexp, p palette) string {
	if m == nil || !p.on {
		return line
	}
	return m.ReplaceAllStringFunc(line, func(s string) string {
		if s == "" {
			return s
		}
		return p.match(s)
	})
}
