package schema

import (
	"testing"

	"tfq/internal/validate"
)

func TestReportOutputMatchesSchema(t *testing.T) {
	for _, strict := range []bool{false, true} {
		r, err := validate.Run("../validate/testdata/vault", strict)
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidateReport(r); err != nil {
			t.Errorf("strict=%v report schema violation: %v", strict, err)
		}
	}
}
