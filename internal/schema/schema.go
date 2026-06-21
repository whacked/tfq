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

//go:embed edges.schema.json
var EdgesSchema []byte

//go:embed hits.schema.json
var HitsSchema []byte

//go:embed report.schema.json
var ReportSchema []byte

var compiled = mustCompile()
var compiledEdges = mustCompileNamed("edges.schema.json", EdgesSchema)
var compiledHits = mustCompileNamed("hits.schema.json", HitsSchema)
var compiledReport = mustCompileNamed("report.schema.json", ReportSchema)

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

func mustCompileNamed(name string, src []byte) *jsonschema.Schema {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(src))
	if err != nil {
		panic(fmt.Sprintf("%s not valid json: %v", name, err))
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(name, doc); err != nil {
		panic(err)
	}
	s, err := c.Compile(name)
	if err != nil {
		panic(err)
	}
	return s
}

func validateAgainst(s *jsonschema.Schema, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
	if err != nil {
		return err
	}
	return s.Validate(inst)
}

// ValidateEdges validates graph edge output against the embedded schema.
// Takes any to avoid importing graph (which would create an import cycle).
func ValidateEdges(edges any) error { return validateAgainst(compiledEdges, edges) }

// ValidateHits validates search hit output against the embedded schema.
// Takes any to avoid importing search (which would create an import cycle).
func ValidateHits(hits any) error { return validateAgainst(compiledHits, hits) }

// ValidateReport validates a validation Report against the embedded schema.
// Takes any to avoid importing validate (which would create an import cycle).
func ValidateReport(report any) error { return validateAgainst(compiledReport, report) }
