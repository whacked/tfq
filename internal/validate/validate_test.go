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
