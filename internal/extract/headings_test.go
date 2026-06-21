package extract

import "testing"

func TestHeadingsMarkdown(t *testing.T) {
	c := "# One\ntext\n### Three\n#nospace\n"
	hs, _ := Headings(c, "markdown")
	if len(hs) != 2 {
		t.Fatalf("got %d headings: %#v", len(hs), hs)
	}
	if hs[0].Level != 1 || hs[0].Text != "One" || hs[0].Line != 1 {
		t.Errorf("h0 = %#v", hs[0])
	}
	if hs[1].Level != 3 || hs[1].Text != "Three" || hs[1].Line != 3 {
		t.Errorf("h1 = %#v", hs[1])
	}
}

func TestHeadingsOrg(t *testing.T) {
	c := "* One\n** Two\n# not a heading in org\n"
	hs, _ := Headings(c, "org")
	if len(hs) != 2 {
		t.Fatalf("got %d: %#v", len(hs), hs)
	}
	if hs[1].Level != 2 || hs[1].Text != "Two" {
		t.Errorf("h1 = %#v", hs[1])
	}
}
