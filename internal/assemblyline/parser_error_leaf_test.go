package assemblyline

import (
	"errors"
	"strings"
	"testing"
)

func TestBoundedSourceParserCorrectionUsesOnlyNestedNonemptyLeaf(t *testing.T) {
	t.Parallel()
	const body = "return left + #;"
	_, err := ValidateJavaScriptFragment("function Value(left)", body)
	correction := requireSourceBodyCorrection(t, err, body)
	if correction.Mutable() != "#" {
		t.Fatalf("mutable parser leaf=%q; want exact nested leaf %q", correction.Mutable(), "#")
	}
	modelInput, err := correction.ModelInput()
	if err != nil {
		t.Fatal(err)
	}
	if modelInput != "What should replace this syntactically invalid span?\n\n#" {
		t.Fatalf("parser correction input=%q", modelInput)
	}
	for _, accepted := range []string{"return", "left", "+", ";"} {
		if strings.Contains(modelInput, accepted) {
			t.Fatalf("parser correction included accepted surrounding child %q: %q", accepted, modelInput)
		}
	}
}

func TestTypeScriptParserCorrectionUsesOnlyAtomicNonemptyLeaf(t *testing.T) {
	t.Parallel()
	const body = "return @;"
	_, err := ParseTypeScriptFunctionBody(
		TypeScriptFunctionContract{Signature: "function Value(): number"},
		body,
	)
	correction := requireSourceBodyCorrection(t, err, body)
	if correction.Mutable() != "@" {
		t.Fatalf("mutable parser leaf=%q; want exact atomic leaf %q", correction.Mutable(), "@")
	}
	modelInput, err := correction.ModelInput()
	if err != nil {
		t.Fatal(err)
	}
	if modelInput != "What should replace this syntactically invalid span?\n\n@" {
		t.Fatalf("parser correction input=%q", modelInput)
	}
	for _, accepted := range []string{"return", ";"} {
		if strings.Contains(modelInput, accepted) {
			t.Fatalf("parser correction included accepted surrounding child %q: %q", accepted, modelInput)
		}
	}
}

func TestParserCorrectionRejectsCompositeAndMissingErrors(t *testing.T) {
	t.Parallel()
	validators := []struct {
		name     string
		validate func(string) error
	}{
		{
			name: "javascript",
			validate: func(body string) error {
				_, err := ValidateJavaScriptFragment("function Value(left, right)", body)
				return err
			},
		},
		{
			name: "typescript",
			validate: func(body string) error {
				_, err := ParseTypeScriptFunctionBody(
					TypeScriptFunctionContract{
						Signature: "function Value(left: number, right: number): number",
					},
					body,
				)
				return err
			},
		},
	}
	cases := []struct {
		name string
		body string
	}{
		{name: "composite", body: "return left @ right;"},
		{name: "missing", body: "return left + ;"},
	}
	for _, validator := range validators {
		validator := validator
		for _, test := range cases {
			test := test
			t.Run(validator.name+"/"+test.name, func(t *testing.T) {
				t.Parallel()
				err := validator.validate(test.body)
				if err == nil {
					t.Fatal("invalid source unexpectedly passed")
				}
				var defect *SourceBodyDefect
				if errors.As(err, &defect) {
					correction, correctionErr := defect.Correction(test.body)
					if correctionErr != nil {
						t.Fatalf("unexpected correction defect=%v correction error=%v", err, correctionErr)
					}
					t.Fatalf("unproven parser span authorized correction over %q", correction.Mutable())
				}
			})
		}
	}
}

func TestParserCorrectionRejectsACompletePreviousBody(t *testing.T) {
	t.Parallel()
	validators := []struct {
		name     string
		validate func(string) error
	}{
		{
			name: "javascript",
			validate: func(body string) error {
				_, err := ValidateJavaScriptFragment("function Value()", body)
				return err
			},
		},
		{
			name: "typescript",
			validate: func(body string) error {
				_, err := ParseTypeScriptFunctionBody(
					TypeScriptFunctionContract{Signature: "function Value(): number"},
					body,
				)
				return err
			},
		},
	}
	for _, validator := range validators {
		validator := validator
		t.Run(validator.name, func(t *testing.T) {
			t.Parallel()
			const body = "@"
			err := validator.validate(body)
			if err == nil {
				t.Fatal("invalid complete body unexpectedly passed")
			}
			var defect *SourceBodyDefect
			if errors.As(err, &defect) {
				t.Fatalf("complete previous body authorized correction: %v", err)
			}
		})
	}
}

func requireSourceBodyCorrection(
	t *testing.T,
	validationErr error,
	body string,
) SourceBodyCorrection {
	t.Helper()
	var defect *SourceBodyDefect
	if !errors.As(validationErr, &defect) {
		t.Fatalf("validation error=%v; want exact source-body defect", validationErr)
	}
	correction, err := defect.Correction(body)
	if err != nil {
		t.Fatal(err)
	}
	return correction
}
