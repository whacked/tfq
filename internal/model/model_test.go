package model

import (
	"encoding/json"
	"testing"
)

func TestFileVitalsJSONKeys(t *testing.T) {
	label := "alias"
	fv := FileVitals{
		Path:        "a.md",
		Ext:         ".md",
		Format:      "markdown",
		Frontmatter: map[string]any{"k": "v"},
		Headings:    []Heading{{Level: 1, Text: "T", Line: 3}},
		Links:       []Link{{Kind: LinkWiki, Target: "x", Label: &label, Line: 8, Col: 4}},
		Markers:     []Marker{{Kind: MarkerHashtag, Value: "tag", Line: 9, Col: 1}},
		Warnings:    []Warning{},
	}
	b, err := json.Marshal(fv)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"path", "ext", "format", "frontmatter", "headings", "links", "markers", "warnings"} {
		if _, ok := got[k]; !ok {
			t.Errorf("missing key %q in %s", k, b)
		}
	}
	links := got["links"].([]any)
	if links[0].(map[string]any)["label"] != "alias" {
		t.Errorf("label wrong: %v", links[0])
	}
}

func TestNilLabelSerializesNull(t *testing.T) {
	b, _ := json.Marshal(Link{Kind: LinkBareURL, Target: "http://x", Line: 1, Col: 1})
	var got map[string]any
	_ = json.Unmarshal(b, &got)
	if v, ok := got["label"]; !ok || v != nil {
		t.Errorf("nil label should serialize as JSON null, got %v ok=%v", v, ok)
	}
}
