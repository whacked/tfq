package schema

import (
	"testing"

	"tfq/internal/query"
)

func TestTypesOutputMatchesSchema(t *testing.T) {
	types := []query.TypeCount{{Type: "note", Count: 5}, {Type: "task", Count: 2}}
	if err := ValidateTypes(types); err != nil {
		t.Errorf("valid types rejected: %v", err)
	}
}

func TestTypesSchemaRejectsBad(t *testing.T) {
	bad := []map[string]any{{"type": "note"}} // missing count
	if err := ValidateTypes(bad); err == nil {
		t.Error("expected schema rejection for missing count")
	}
}
