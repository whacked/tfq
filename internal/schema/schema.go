package schema

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"tfq/internal/model"
)

//go:embed filevitals.schema.json
var FileVitalsSchema []byte

var compiled = mustCompile()

func mustCompile() *jsonschema.Schema {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(FileVitalsSchema))
	if err != nil {
		panic(fmt.Sprintf("schema not valid json: %v", err))
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("filevitals.schema.json", doc); err != nil {
		panic(err)
	}
	s, err := c.Compile("filevitals.schema.json")
	if err != nil {
		panic(err)
	}
	return s
}

// validateJSON validates a raw JSON document against the FileVitals schema.
func validateJSON(b []byte) error {
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("invalid json: %w", err)
	}
	return compiled.Validate(inst)
}

// ValidateFileVitals marshals fv and validates it against the embedded schema.
func ValidateFileVitals(fv model.FileVitals) error {
	b, err := json.Marshal(fv)
	if err != nil {
		return err
	}
	return validateJSON(b)
}
