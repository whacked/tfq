package main

import (
	"regexp"
	"strings"
	"testing"
)

func TestDecideColor(t *testing.T) {
	cases := []struct {
		mode           string
		noColor, isTTY bool
		want           bool
	}{
		{"auto", false, true, true},   // TTY, no NO_COLOR -> color
		{"auto", false, false, false}, // piped -> no color
		{"auto", true, true, false},   // NO_COLOR set -> no color
		{"always", true, false, true}, // forced on even piped + NO_COLOR
		{"never", false, true, false}, // forced off even on a TTY
	}
	for _, c := range cases {
		if got := decideColor(c.mode, c.noColor, c.isTTY); got != c.want {
			t.Errorf("decideColor(%q,noColor=%v,isTTY=%v)=%v, want %v", c.mode, c.noColor, c.isTTY, got, c.want)
		}
	}
}

func TestPaletteWrap(t *testing.T) {
	on := palette{on: true}
	if !strings.Contains(on.path("x"), "\x1b[35;1m") || !strings.HasSuffix(on.path("x"), "\x1b[0m") {
		t.Errorf("color on should wrap: %q", on.path("x"))
	}
	off := palette{on: false}
	if off.path("x") != "x" {
		t.Errorf("color off should be plain, got %q", off.path("x"))
	}
	if on.path("") != "" {
		t.Error("empty string should never get codes")
	}
}

func TestHighlight(t *testing.T) {
	m := regexp.MustCompile("needle")
	out := highlight("a needle here", m, palette{on: true})
	if !strings.Contains(out, "\x1b[1;31mneedle\x1b[0m") {
		t.Errorf("match not highlighted: %q", out)
	}
	if highlight("a needle", m, palette{on: false}) != "a needle" {
		t.Error("color off should not highlight")
	}
	if highlight("plain", nil, palette{on: true}) != "plain" {
		t.Error("nil matcher should not highlight")
	}
}
