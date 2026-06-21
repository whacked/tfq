package extract

import "testing"

func TestFrontmatterParses(t *testing.T) {
	c := "---\ntitle: Hi\ntags: [a, b]\n---\n# Heading\nbody\n"
	fm, body, warns := Frontmatter(c)
	if len(warns) != 0 {
		t.Fatalf("unexpected warnings: %v", warns)
	}
	if fm["title"] != "Hi" {
		t.Errorf("title = %v", fm["title"])
	}
	// body must preserve line count: heading was on line 5, still line 5
	l, _ := lineCol(body, indexOf(body, "# Heading"))
	if l != 5 {
		t.Errorf("heading line = %d, want 5", l)
	}
	// frontmatter region blanked
	if containsLine(body, "title: Hi") {
		t.Errorf("frontmatter not blanked: %q", body)
	}
}

func TestFrontmatterNone(t *testing.T) {
	c := "# Just a heading\nno frontmatter\n"
	fm, body, warns := Frontmatter(c)
	if len(fm) != 0 {
		t.Errorf("expected empty fm, got %v", fm)
	}
	if body != c {
		t.Errorf("body should be unchanged")
	}
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
}

func TestFrontmatterMalformed(t *testing.T) {
	c := "---\ntitle: : : bad\n  - nope\n---\nbody\n"
	fm, _, warns := Frontmatter(c)
	if len(fm) != 0 {
		t.Errorf("malformed fm should yield empty map, got %v", fm)
	}
	if len(warns) == 0 {
		t.Errorf("expected a warning for malformed yaml")
	}
}

// test helpers
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
func containsLine(s, sub string) bool { return indexOf(s, sub) >= 0 }
