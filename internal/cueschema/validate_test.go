package cueschema

import (
	"path/filepath"
	"testing"
)

func loadTestSchema(t *testing.T) *Schema {
	t.Helper()
	s, err := Load(filepath.Join("testdata", ".tfq.cue"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestValidateValid(t *testing.T) {
	s := loadTestSchema(t)
	if v := s.Validate(map[string]any{"status": "pending"}); len(v) != 0 {
		t.Errorf("valid frontmatter produced violations: %#v", v)
	}
	// extra fields are tolerated
	if v := s.Validate(map[string]any{"status": "completed", "extra": "x"}); len(v) != 0 {
		t.Errorf("extra field should be tolerated: %#v", v)
	}
}

func TestValidateBadEnum(t *testing.T) {
	s := loadTestSchema(t)
	v := s.Validate(map[string]any{"status": "bogus"})
	if len(v) == 0 {
		t.Error("bad enum value should produce a violation")
	}
}

func TestValidateMissingRequired(t *testing.T) {
	s := loadTestSchema(t)
	// status is required (no ?); omit it
	v := s.Validate(map[string]any{"priority": "low"})
	if len(v) == 0 {
		t.Error("missing required status should produce a violation")
	}
}
