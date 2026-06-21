package scan

import "testing"

func TestCollect(t *testing.T) {
	recs, warns, err := Collect("testdata/vault")
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
	if len(recs) != 2 {
		t.Fatalf("expected 2 records (md + org), got %d: %#v", len(recs), recs)
	}
	// sorted by relative path: "note-a.md" before "sub/note-b.org"
	if recs[0].Path != "note-a.md" {
		t.Errorf("rec0 path = %q, want note-a.md", recs[0].Path)
	}
	if recs[1].Path != "sub/note-b.org" {
		t.Errorf("rec1 path = %q, want sub/note-b.org", recs[1].Path)
	}
	if recs[1].Format != "org" {
		t.Errorf("rec1 format = %q, want org", recs[1].Format)
	}
}
