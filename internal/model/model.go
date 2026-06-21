package model

// Link kinds.
const (
	LinkMarkdown = "markdown"
	LinkWiki     = "wiki"
	LinkEmbed    = "embed"
	LinkOrg      = "org"
	LinkAutolink = "autolink"
	LinkBareURL  = "bare-url"
)

// Marker kinds.
const (
	MarkerHashtag     = "hashtag"
	MarkerOrgTag      = "org-tag"
	MarkerAngle       = "angle"
	MarkerDoubleAngle = "double-angle"
)

// Position is a 1-based line/column location.
type Position struct {
	Line int `json:"line"`
	Col  int `json:"col"`
}

// Heading is a section heading (markdown # or org *).
type Heading struct {
	Level int    `json:"level"`
	Text  string `json:"text"`
	Line  int    `json:"line"`
}

// Link is any cross-reference found in the body. Label is nil when absent.
type Link struct {
	Kind   string  `json:"kind"`
	Target string  `json:"target"`
	Label  *string `json:"label"`
	Line   int     `json:"line"`
	Col    int     `json:"col"`
}

// Marker is a hashtag, org tag, or angle-bracket phrase.
type Marker struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
	Line  int    `json:"line"`
	Col   int    `json:"col"`
}

// Warning is a non-fatal extraction issue.
type Warning struct {
	Module  string `json:"module"`
	Message string `json:"message"`
}

// FileVitals is the comprehensive per-file output contract.
type FileVitals struct {
	Path        string         `json:"path"`
	Ext         string         `json:"ext"`
	Format      string         `json:"format"`
	Frontmatter map[string]any `json:"frontmatter"`
	Headings    []Heading      `json:"headings"`
	Links       []Link         `json:"links"`
	Markers     []Marker       `json:"markers"`
	Warnings    []Warning      `json:"warnings"`
}
