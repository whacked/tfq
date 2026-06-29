package validate

import "testing"

func hasFinding(r Report, path, sev string) bool {
	for _, f := range r.Findings {
		if f.Path == path && f.Severity == sev {
			return true
		}
	}
	return false
}

func TestRunLiberal(t *testing.T) {
	r, err := Run("testdata/vault", false)
	if err != nil {
		t.Fatal(err)
	}
	if !r.OK {
		t.Errorf("liberal run should be OK even with findings")
	}
	// bad.md has a bad enum + a dangling dep + a dangling wiki link -> warnings
	if !hasFinding(r, "bad.md", "warning") {
		t.Errorf("expected warnings on bad.md: %#v", r.Findings)
	}
	// good.md is clean
	if hasFinding(r, "good.md", "warning") || hasFinding(r, "good.md", "error") {
		t.Errorf("good.md should have no findings: %#v", r.Findings)
	}
}

func TestRunStrict(t *testing.T) {
	r, err := Run("testdata/vault", true)
	if err != nil {
		t.Fatal(err)
	}
	if r.OK {
		t.Errorf("strict run should fail given bad.md")
	}
	if !hasFinding(r, "bad.md", "error") {
		t.Errorf("expected error severity on bad.md in strict mode: %#v", r.Findings)
	}
}

// File validates a single file's frontmatter against an explicitly named schema
// (cue vet semantics: any violation is an error). This is the tfq replacement
// for `cue vet` in agent-resources' validate-frontmatter.sh.
func TestFileAgainstExplicitSchema(t *testing.T) {
	ok, err := File("testdata/note-ok.md", "testdata/notes.template.md")
	if err != nil {
		t.Fatal(err)
	}
	if !ok.OK || len(ok.Findings) != 0 {
		t.Errorf("note-ok should be valid: %#v", ok.Findings)
	}

	bad, err := File("testdata/note-bad.md", "testdata/notes.template.md")
	if err != nil {
		t.Fatal(err)
	}
	if bad.OK {
		t.Error("note-bad violates the schema, should not be OK")
	}
	if !hasFinding(bad, "testdata/note-bad.md", "error") {
		t.Errorf("expected an error finding on note-bad: %#v", bad.Findings)
	}
}

// RunWith validates a whole collection against an explicit schema instead of the
// discovered .tfq.cue.
func TestRunWithSchemaOverride(t *testing.T) {
	// good.md has no author; the override requires one, so it must now flag good.md.
	r, err := RunWith("testdata/vault", true, "testdata/require-author.cue")
	if err != nil {
		t.Fatal(err)
	}
	if !hasFinding(r, "good.md", "error") {
		t.Errorf("override schema should flag good.md (missing author): %#v", r.Findings)
	}
}
