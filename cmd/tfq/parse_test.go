package main

import (
	"reflect"
	"testing"
)

func TestParseDefaultSearch(t *testing.T) {
	inv, err := parse([]string{"battery", "supply", "chain"})
	if err != nil {
		t.Fatal(err)
	}
	if inv.Mode != ModeSearch || inv.Selector != "battery supply chain" {
		t.Errorf("got mode=%v selector=%q", inv.Mode, inv.Selector)
	}
}

func TestParseFlagsAndSelectorInterleaved(t *testing.T) {
	inv, err := parse([]string{"--tag", "battery", "supply", "chain", "-i"})
	if err != nil {
		t.Fatal(err)
	}
	if inv.Selector != "supply chain" || !inv.IgnoreCase {
		t.Errorf("selector=%q ignoreCase=%v", inv.Selector, inv.IgnoreCase)
	}
	if !reflect.DeepEqual(inv.Tags, []string{"battery"}) {
		t.Errorf("tags=%#v", inv.Tags)
	}
}

func TestParseModeAndAliases(t *testing.T) {
	if inv, _ := parse([]string{"--done", "task-1"}); inv.Mode != ModeSet || inv.Status != "done" || inv.Selector != "task-1" {
		t.Errorf("--done parsed wrong: %#v", inv)
	}
	if inv, _ := parse([]string{"--task", "do-it"}); inv.Mode != ModeNew || inv.Type != "task" {
		t.Errorf("--task parsed wrong: %#v", inv)
	}
	if inv, _ := parse([]string{"--backlinks", "x"}); inv.Mode != ModeLinks || !inv.Inbound {
		t.Errorf("--backlinks parsed wrong: %#v", inv)
	}
}

func TestParseExplicitQueryAndDashDash(t *testing.T) {
	if inv, _ := parse([]string{"-e", "-foo"}); inv.Selector != "-foo" {
		t.Errorf("-e selector=%q", inv.Selector)
	}
	if inv, _ := parse([]string{"--", "-bar"}); inv.Selector != "-bar" {
		t.Errorf("-- selector=%q", inv.Selector)
	}
}

func TestParseRepeatTagsAndFields(t *testing.T) {
	inv, _ := parse([]string{"--new", "x", "--tag", "a", "--tag", "b", "--field", "k=v"})
	if !reflect.DeepEqual(inv.Tags, []string{"a", "b"}) {
		t.Errorf("tags=%#v", inv.Tags)
	}
	if inv.Fields["k"] != "v" {
		t.Errorf("fields=%#v", inv.Fields)
	}
}

func TestParseErrors(t *testing.T) {
	if _, err := parse([]string{"--show", "--links", "x"}); err == nil {
		t.Error("expected error for two modes")
	}
	if _, err := parse([]string{"--bogus"}); err == nil {
		t.Error("expected error for unknown flag")
	}
	if _, err := parse([]string{"--type"}); err == nil {
		t.Error("expected error for missing value")
	}
	if _, err := parse([]string{"--limit", "x"}); err == nil {
		t.Error("expected error for non-integer limit")
	}
}

func TestParseHeadingDefaultTrue(t *testing.T) {
	if inv, _ := parse([]string{"x"}); !inv.Heading {
		t.Error("heading should default true")
	}
	if inv, _ := parse([]string{"x", "--no-heading"}); inv.Heading {
		t.Error("--no-heading should clear heading")
	}
}

func TestParseColor(t *testing.T) {
	if inv, _ := parse([]string{"x"}); inv.Color != "auto" {
		t.Errorf("default color=%q, want auto", inv.Color)
	}
	if inv, _ := parse([]string{"x", "--color", "always"}); inv.Color != "always" {
		t.Errorf("--color always not parsed: %q", inv.Color)
	}
	if inv, _ := parse([]string{"x", "--no-color"}); inv.Color != "never" {
		t.Errorf("--no-color should set never: %q", inv.Color)
	}
	if _, err := parse([]string{"x", "--color", "bogus"}); err == nil {
		t.Error("invalid --color should error")
	}
}

func TestParseInAndTypes(t *testing.T) {
	inv, err := parse([]string{"battery", "--in", "heading", "--in", "tag"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(inv.In, []string{"heading", "tag"}) {
		t.Errorf("In = %#v", inv.In)
	}
	if _, err := parse([]string{"x", "--in", "bullet"}); err == nil {
		t.Error("invalid --in should error")
	}
	if inv, _ := parse([]string{"--types"}); inv.Mode != ModeTypes {
		t.Errorf("--types mode = %v", inv.Mode)
	}
}

func TestParsePorcelainTaskFlags(t *testing.T) {
	inv, err := parse([]string{"--task", "build", "--priority", "high", "--effort", "small",
		"--parent", "001", "--depends-on", "002,003", "--depends-on", "004"})
	if err != nil {
		t.Fatal(err)
	}
	if inv.Fields["priority"] != "high" || inv.Fields["effort"] != "small" || inv.Fields["parent"] != "001" {
		t.Errorf("scalar porcelain flags not mapped to Fields: %#v", inv.Fields)
	}
	if !reflect.DeepEqual(inv.DependsOn, []string{"002", "003", "004"}) {
		t.Errorf("--depends-on should split on comma and accumulate: %#v", inv.DependsOn)
	}
}

func TestParseTitle(t *testing.T) {
	inv, err := parse([]string{"--task", "--title", "Audit Vendors"})
	if err != nil {
		t.Fatal(err)
	}
	if inv.Title != "Audit Vendors" {
		t.Errorf("Title = %q", inv.Title)
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Audit Vendors":      "audit-vendors",
		"  Spaces & Punct! ": "spaces-punct",
		"already-a-slug":     "already-a-slug",
		"CamelCase":          "camelcase",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseSchema(t *testing.T) {
	inv, err := parse([]string{"--validate", "note.md", "--schema", "tpl.cue"})
	if err != nil {
		t.Fatal(err)
	}
	if inv.Mode != ModeValidate || inv.Selector != "note.md" || inv.Schema != "tpl.cue" {
		t.Errorf("got mode=%v selector=%q schema=%q", inv.Mode, inv.Selector, inv.Schema)
	}
}
