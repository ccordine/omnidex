package assemblyline

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestDeclarationArtifactBoundaryIsOnePathBlindSemanticLeaf(t *testing.T) {
	t.Parallel()

	input := DeclarationArtifactBoundaryInput{
		RequirementQuote: "func Normalize(input string) string has an independent artifact boundary",
		GoSignature:      "func Normalize(input string) string",
		DeclarationID:    "DECLARATION_1",
	}
	job, err := NewDeclarationArtifactBoundaryJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	encodedSchema, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{input.RequirementQuote, input.GoSignature, input.DeclarationID} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("boundary prompt omitted exact authority %q:\n%s", required, prompt)
		}
	}
	for label, projection := range map[string]string{
		"prompt": prompt,
		"schema": string(encodedSchema),
	} {
		lower := strings.ToLower(projection)
		for _, forbidden := range []string{
			"create_file", "delete_file", "write_file", "rename_file", "move_file",
			"filesystem", "create ", "delete ", "modify ", "rename ", "move ", "write ",
			" path", " filename", " operation", " action", " command", " patch", " workspace", " tree",
			`"path"`, `"filename"`, `"operation"`, `"action"`, `"command"`, `"patch"`,
		} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("%s exposed forbidden repository authority %q: %s", label, forbidden, projection)
			}
		}
	}
	properties := schema["properties"].(map[string]any)
	if len(properties) != 3 || properties["declaration_id"].(map[string]any)["const"] != input.DeclarationID {
		t.Fatalf("boundary schema is not closed over one declaration: %#v", schema)
	}
	decision := DeclarationArtifactBoundaryDecision{
		Schema: DeclarationArtifactBoundarySchemaV1, DeclarationID: input.DeclarationID,
		Boundary: DeclarationBoundaryIndependentArtifact,
	}
	if err := decision.ValidateFor(input); err != nil {
		t.Fatal(err)
	}
	decision.Boundary = DeclarationArtifactBoundary("create_file")
	if err := decision.ValidateFor(input); err == nil {
		t.Fatal("filesystem operation was accepted as a declaration boundary")
	}
	for _, typ := range []reflect.Type{reflect.TypeOf(input), reflect.TypeOf(decision)} {
		for _, forbidden := range []string{"Path", "File", "Filename", "Operation", "Action", "Command", "Patch", "Tree", "Workspace"} {
			if _, exists := typ.FieldByName(forbidden); exists {
				t.Fatalf("%s exposes forbidden field %q", typ.Name(), forbidden)
			}
		}
	}
}

func TestDeclarationArtifactBoundaryRejectsPhysicalIdentityAndForgedDeclaration(t *testing.T) {
	t.Parallel()

	valid := DeclarationArtifactBoundaryInput{
		RequirementQuote: "func Normalize(input string) string has an independent artifact boundary",
		GoSignature:      "func Normalize(input string) string",
		DeclarationID:    "DECLARATION_1",
	}
	for name, mutate := range map[string]func(*DeclarationArtifactBoundaryInput){
		"physical identity": func(input *DeclarationArtifactBoundaryInput) {
			input.RequirementQuote = "Put func Normalize(input string) string in internal/text/normalize.go"
		},
		"multiline signature": func(input *DeclarationArtifactBoundaryInput) {
			input.GoSignature += "\nfunc Other()"
		},
		"unbounded declaration": func(input *DeclarationArtifactBoundaryInput) {
			input.DeclarationID = "normalize.go"
		},
	} {
		t.Run(name, func(t *testing.T) {
			input := valid
			mutate(&input)
			if _, err := NewDeclarationArtifactBoundaryJob(input); err == nil {
				t.Fatalf("accepted invalid declaration boundary input: %+v", input)
			}
		})
	}

	decision := DeclarationArtifactBoundaryDecision{
		Schema: DeclarationArtifactBoundarySchemaV1, DeclarationID: "DECLARATION_2",
		Boundary: DeclarationBoundaryNone,
	}
	if err := decision.ValidateFor(valid); err == nil {
		t.Fatal("decision selected a declaration outside its immutable input")
	}
}

func TestDeclarationArtifactBoundaryAllowsPathBlindQualifiedGoSymbols(t *testing.T) {
	t.Parallel()

	_, err := NewDeclarationArtifactBoundaryJob(DeclarationArtifactBoundaryInput{
		RequirementQuote: "func Matches(err error) bool has an independent artifact boundary using errors.Is semantics",
		GoSignature:      "func Matches(err error) bool",
		DeclarationID:    "DECLARATION_1",
	})
	if err != nil {
		t.Fatalf("path-blind qualified Go symbol was mistaken for physical identity: %v", err)
	}
}

func TestDeclarationArtifactBoundaryAcceptsCodeCanonicalSignatureAuthority(t *testing.T) {
	t.Parallel()

	_, err := NewDeclarationArtifactBoundaryJob(DeclarationArtifactBoundaryInput{
		RequirementQuote: "func Normalize(input string)string has an independent artifact boundary",
		GoSignature:      "func Normalize(input string) string",
		DeclarationID:    "DECLARATION_1",
	})
	if err != nil {
		t.Fatalf("canonical Go signature was required to reproduce source spacing: %v", err)
	}
}
