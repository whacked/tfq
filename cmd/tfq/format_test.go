package main

import (
	"regexp"
	"strings"
	"testing"

	"tfq/internal/graph"
	"tfq/internal/query"
	"tfq/internal/search"
)

func TestFormatHitsHeading(t *testing.T) {
	hits := []search.Hit{
		{Path: "a.md", Line: 12, Text: "model battery"},
		{Path: "a.md", Line: 37, Text: "supply risk"},
		{Path: "b.md", Line: 8, Text: "cathode"},
	}
	out := formatHits(hits, true, nil, palette{})
	if !strings.Contains(out, "a.md\n12: model battery\n37: supply risk") {
		t.Errorf("heading output wrong:\n%s", out)
	}
	flat := formatHits(hits, false, nil, palette{})
	if !strings.Contains(flat, "a.md:12:model battery") {
		t.Errorf("no-heading output wrong:\n%s", flat)
	}
}

func TestFormatHitsColor(t *testing.T) {
	hits := []search.Hit{{Path: "a.md", Line: 1, Text: "the needle here"}}
	out := formatHits(hits, true, regexp.MustCompile("needle"), palette{on: true})
	if !strings.Contains(out, "\x1b[35;1ma.md\x1b[0m") {
		t.Errorf("path not colored:\n%q", out)
	}
	if !strings.Contains(out, "\x1b[1;31mneedle\x1b[0m") {
		t.Errorf("match not highlighted:\n%q", out)
	}
}

func TestFilesAndCounts(t *testing.T) {
	hits := []search.Hit{{Path: "a.md", Line: 1}, {Path: "a.md", Line: 2}, {Path: "b.md", Line: 1}}
	if got := filesOf(hits); len(got) != 2 || got[0] != "a.md" {
		t.Errorf("filesOf = %#v", got)
	}
	c := countsOf(hits)
	if len(c) != 2 || c[0].Path != "a.md" || c[0].Count != 2 {
		t.Errorf("countsOf = %#v", c)
	}
}

func TestFormatListBlock(t *testing.T) {
	out := formatList([]query.ListItem{{Path: "x.md", Type: "task", Status: "pending", Tags: []string{"a"}, Title: "Do X"}}, palette{})
	if !strings.Contains(out, "x.md  task pending #a") || !strings.Contains(out, "title: Do X") {
		t.Errorf("list block wrong:\n%s", out)
	}
}

func TestFormatLinksBothDirections(t *testing.T) {
	out := formatLinks("a.md",
		[]graph.Edge{{From: "a.md", Kind: "wiki", Raw: "b", To: "b.md"}},
		[]string{"c.md"}, true, true, palette{})
	if !strings.Contains(out, "# outbound links") || !strings.Contains(out, "==> b.md") {
		t.Errorf("missing outbound:\n%s", out)
	}
	if !strings.Contains(out, "# inbound links") || !strings.Contains(out, "<== c.md") {
		t.Errorf("missing inbound:\n%s", out)
	}
}

func TestFormatTagsIndex(t *testing.T) {
	out := formatTagsIndex([]query.TagCount{{Tag: "battery", Count: 42}}, palette{})
	if !strings.Contains(out, "battery") || !strings.Contains(out, "42") {
		t.Errorf("tags index wrong:\n%s", out)
	}
}

func TestFormatHitsKindLabel(t *testing.T) {
	hits := []search.Hit{{Path: "a.md", Line: 1, Text: "# Battery", Kinds: []string{"heading"}}}
	out := formatHits(hits, true, nil, palette{})
	if !strings.Contains(out, "[heading]") {
		t.Errorf("expected kind label:\n%s", out)
	}
	// prose hit (no kinds) -> no brackets
	plain := formatHits([]search.Hit{{Path: "a.md", Line: 2, Text: "prose"}}, true, nil, palette{})
	if strings.Contains(plain, "[") {
		t.Errorf("prose hit should have no label:\n%s", plain)
	}
}

func TestFormatTypesIndex(t *testing.T) {
	out := formatTypesIndex([]query.TypeCount{{Type: "note", Count: 41}}, palette{})
	if !strings.Contains(out, "# types") || !strings.Contains(out, "note") || !strings.Contains(out, "41") {
		t.Errorf("types index wrong:\n%s", out)
	}
}
