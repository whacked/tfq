package registry

import "testing"

func TestFormatFor(t *testing.T) {
	cases := map[string]string{
		".md":       "markdown",
		".markdown": "markdown",
		"md":        "markdown",
		".MD":       "markdown",
		".org":      "org",
		".txt":      "text",
		"":          "text",
		".rst":      "text",
	}
	for in, want := range cases {
		if got := FormatFor(in); got != want {
			t.Errorf("FormatFor(%q) = %q, want %q", in, got, want)
		}
	}
}
