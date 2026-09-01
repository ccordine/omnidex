package assemblyline

import (
	"errors"
	"testing"
)

func TestTypeScriptForbiddenIdentifierLocatesOnlyDirectValueReference(t *testing.T) {
	t.Parallel()
	const body = "return window + input;"
	_, err := ParseTypeScriptFunctionBody(
		TypeScriptFunctionContract{
			Signature: "function Value(input: number): number",
			Policy: SourceFunctionPolicy{
				ForbiddenIdentifiers: []string{"window"},
			},
		},
		body,
	)
	var defect *SourceBodyDefect
	if !errors.As(err, &defect) {
		t.Fatalf("direct value-reference error = %v; want exact source-body defect", err)
	}
	mutable, mutableErr := defect.Mutable(body)
	if mutableErr != nil {
		t.Fatal(mutableErr)
	}
	if mutable != "window" {
		t.Fatalf("direct value-reference span = %q; want window", mutable)
	}
}

func TestTypeScriptForbiddenIdentifierIgnoresPropertyAndTypeNames(t *testing.T) {
	t.Parallel()
	const body = `type window = { value: number };
const record = { window: input };
return record.window;`
	if _, err := ParseTypeScriptFunctionBody(
		TypeScriptFunctionContract{
			Signature: "function Value(input: number): number",
			Policy: SourceFunctionPolicy{
				ForbiddenIdentifiers: []string{"window"},
			},
		},
		body,
	); err != nil {
		t.Fatalf("property/type spelling acquired value-reference authority: %v", err)
	}
}

func TestTypeScriptForbiddenBindingFailsWithoutCorrectionSpan(t *testing.T) {
	t.Parallel()
	const body = "const window = input;\nreturn window;"
	_, err := ParseTypeScriptFunctionBody(
		TypeScriptFunctionContract{
			Signature: "function Value(input: number): number",
			Policy: SourceFunctionPolicy{
				ForbiddenIdentifiers: []string{"window"},
			},
		},
		body,
	)
	if err == nil {
		t.Fatal("forbidden binding unexpectedly passed policy validation")
	}
	var defect *SourceBodyDefect
	if errors.As(err, &defect) {
		t.Fatalf("forbidden binding authorized a value correction span: %v", err)
	}
}
