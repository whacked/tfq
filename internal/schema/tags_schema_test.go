package schema

import (
	"testing"

	"tfq/internal/query"
)

func TestTagsOutputMatchesSchema(t *testing.T) {
	tags := []query.TagCount{{Tag: "x", Count: 2}, {Tag: "y", Count: 1}}
	if err := ValidateTags(tags); err != nil {
		t.Errorf("valid tags rejected: %v", err)
	}
}

func TestTagsSchemaRejectsBad(t *testing.T) {
	bad := []map[string]any{{"tag": "x"}} // missing count
	if err := ValidateTags(bad); err == nil {
		t.Error("expected schema rejection for missing count")
	}
}
