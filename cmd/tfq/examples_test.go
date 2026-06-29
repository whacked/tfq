package main

import (
	"strings"
	"testing"
)

// helpFixture builds the exact sample collection the yoked examples document.
// The line numbers in examples.go Want strings depend on this content.
func helpFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustWrite(t, dir, "battery.md", "---\ntype: note\ntags: [power, supply-chain]\n---\n# Battery supply risk\ntracking the #power rollout\nsee [[cells]] for the spec\nthe battery degrades under load\n")
	mustWrite(t, dir, "cells.md", "---\ntype: note\ntags: [power]\n---\n# Cell chemistry\nnotes on [[battery]] internals\n")
	mustWrite(t, dir, "task-audit.md", "---\ntype: task\nid: \"001\"\nstatus: pending\npriority: high\n---\n# Audit battery vendors\n")
	return dir
}

// The extended-help examples must stay true: run each against the sample
// collection and assert the output still equals the documented Want. This yokes
// `tfq --examples` to real behavior — a format change here fails the build.
func TestHelpExamplesAreAccurate(t *testing.T) {
	dir := helpFixture(t)
	for _, ex := range examples {
		args := append([]string{"--root", dir}, ex.Args...)
		out, code := run(args)
		if code != 0 {
			t.Errorf("example `tfq %s` exited %d:\n%s", strings.Join(ex.Args, " "), code, out)
			continue
		}
		if strings.TrimRight(out, "\n") != strings.TrimRight(ex.Want, "\n") {
			t.Errorf("example `tfq %s` drifted:\n--- got ---\n%q\n--- want ---\n%q",
				strings.Join(ex.Args, " "), out, ex.Want)
		}
	}
}

func TestVerboseHelpRendersExamples(t *testing.T) {
	out, code := run([]string{"--help", "--verbose"})
	if code != 0 {
		t.Fatalf("--help --verbose exited %d", code)
	}
	if !contains(out, "$ tfq battery") || !contains(out, "[heading]") {
		t.Errorf("extended help should render yoked examples:\n%s", out)
	}
	if !contains(out, "supersession scope") && !contains(out, "Supersession scope") {
		t.Errorf("extended help should explain supersession scope:\n%s", out)
	}
	out2, code2 := run([]string{"--examples"})
	if code2 != 0 || out2 != out {
		t.Errorf("--examples should equal --help --verbose")
	}
}

func TestPlainHelpIsShort(t *testing.T) {
	out, _ := run([]string{"--help"})
	if contains(out, "$ tfq battery") {
		t.Errorf("plain --help should be the short usage, not the extended guide with examples")
	}
}
