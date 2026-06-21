package cueschema

import (
	"path/filepath"
	"testing"
)

// Exercises a richer real-world shorthand (the agent-resources reports schema):
// regex constraints (=~), conjunctions (string & =~...), enums, and list types.
func loadReports(t *testing.T) *Schema {
	t.Helper()
	s, err := Load(filepath.Join("testdata", "reports.cue"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestRichSchemaValidDoc(t *testing.T) {
	s := loadReports(t)
	fm := map[string]any{
		"id":         "001",
		"status":     "accepted",
		"date":       "2026-06-22",
		"intent":     "normative",
		"author":     "agent",
		"tags":       []any{"bandgap", "sim"},
		"references": []any{"https://x"},
		"supersedes": "000",
	}
	if v := s.Validate(fm); len(v) != 0 {
		t.Errorf("valid report rejected: %#v", v)
	}
	// all fields optional -> an empty doc is valid (liberal)
	if v := s.Validate(map[string]any{}); len(v) != 0 {
		t.Errorf("empty doc should be valid: %#v", v)
	}
	// date with time component matches the regex too
	if v := s.Validate(map[string]any{"date": "2026-06-22 13:45:01.123"}); len(v) != 0 {
		t.Errorf("timestamped date rejected: %#v", v)
	}
}

func TestRichSchemaRegexViolation(t *testing.T) {
	s := loadReports(t)
	// date that does not match the YYYY-MM-DD regex -> violation
	if v := s.Validate(map[string]any{"date": "June 22, 2026"}); len(v) == 0 {
		t.Error("badly-formatted date should violate the =~ regex constraint")
	}
}

func TestRichSchemaEnumViolation(t *testing.T) {
	s := loadReports(t)
	if v := s.Validate(map[string]any{"status": "totally-made-up"}); len(v) == 0 {
		t.Error("invalid status enum should violate")
	}
	if v := s.Validate(map[string]any{"intent": "sideways"}); len(v) == 0 {
		t.Error("invalid intent enum should violate")
	}
}

func TestRichSchemaTypeViolation(t *testing.T) {
	s := loadReports(t)
	// tags must be a list of strings; give it a number
	if v := s.Validate(map[string]any{"tags": 42}); len(v) == 0 {
		t.Error("non-list tags should violate")
	}
}
