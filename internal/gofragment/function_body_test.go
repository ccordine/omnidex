package gofragment

import (
	"errors"
	"strings"
	"testing"
)

func TestParseNewFunctionBodyUsesCodeOwnedDeclaration(t *testing.T) {
	t.Parallel()
	signature := "func Sum(left, right int) int"
	source, err := ParseNewFunctionBody(
		signature,
		nil,
		"total := left + right\rreturn total\r\n",
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(source, signature) != 1 || strings.ContainsRune(source, '\r') {
		t.Fatalf("code-assembled Go source=%q", source)
	}
	repeated, err := ParseNewFunctionBody(
		signature,
		nil,
		signature+" { return left + right }",
	)
	if err != nil {
		t.Fatalf("extract complete Go declaration: %v", err)
	}
	if strings.Count(repeated, signature) != 1 ||
		!strings.Contains(repeated, "return left + right") {
		t.Fatalf("complete Go declaration extracted source=%q", repeated)
	}
}

func TestParseNewFunctionBodyExtractsOneFencedDeclaration(t *testing.T) {
	t.Parallel()
	source, err := ParseNewFunctionBody(
		"func Sum(left, right int) int",
		nil,
		"Here is the implementation.\n```go\nfunc Other(unused string) string { return left + right }\n```",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(source, "func Sum(left, right int) int") ||
		!strings.Contains(source, "return left + right") || strings.Contains(source, "Other") {
		t.Fatalf("code-owned Go declaration extraction=%q", source)
	}
}

func TestParseNewFunctionBodyRejectsAmbiguousFencesWithoutSpanAuthority(t *testing.T) {
	t.Parallel()
	_, err := ParseNewFunctionBody(
		"func Value() int",
		nil,
		"```go\nfunc One() int { return 1 }\n```\n```go\nfunc Two() int { return 2 }\n```",
	)
	if err == nil || !strings.Contains(err.Error(), "2 fenced regions") {
		t.Fatalf("ambiguous Go extraction error=%v", err)
	}
	var violation *BodySpanViolation
	if errors.As(err, &violation) {
		t.Fatalf("ambiguous Go extraction authorized correction: %v", err)
	}
}

func TestParseNewFunctionBodyRejectsLaterAndOutOfScopeLocals(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "later declaration",
			body: "return total\ntotal := left",
			want: "total",
		},
		{
			name: "sibling block declaration",
			body: "if true { hidden := left; _ = hidden }\nreturn hidden",
			want: "hidden",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseNewFunctionBody(
				"func Value(left int) int", nil, test.body,
			)
			var violation *BodySpanViolation
			if !errors.As(err, &violation) {
				t.Fatalf("scope error=%v; want located body violation", err)
			}
			if got := test.body[violation.StartByte:violation.EndByte]; got != test.want {
				t.Fatalf("failed span=%q; want %q", got, test.want)
			}
		})
	}
}

func TestParseNewFunctionBodyDoesNotGrantCapabilityParameterNames(t *testing.T) {
	t.Parallel()
	const body = "return privateInput"
	_, err := ParseNewFunctionBody(
		"func Value() int",
		[]string{"func Available(privateInput int) int"},
		body,
	)
	var violation *BodySpanViolation
	if !errors.As(err, &violation) {
		t.Fatalf("capability parameter error=%v; want located body violation", err)
	}
	if got := body[violation.StartByte:violation.EndByte]; got != "privateInput" {
		t.Fatalf("failed span=%q; want privateInput", got)
	}
	if _, err := ParseNewFunctionBody(
		"func Value() int",
		[]string{"func Available(privateInput int) int"},
		"return Available(1)",
	); err != nil {
		t.Fatalf("public capability name was not usable: %v", err)
	}
}

func TestParseFunctionDoesNotGrantNamesFromPreviousBody(t *testing.T) {
	t.Parallel()
	contract := Contract{
		Signature: "func Value() int",
		Current:   "func Value() int { hidden := 1; return hidden }",
	}
	_, err := ParseFunction(contract, "func Value() int { return hidden }")
	var violation *UndeclaredIdentifierViolation
	if !errors.As(err, &violation) || violation.Name != "hidden" {
		t.Fatalf("previous-body authority error=%v; want hidden identifier violation", err)
	}
}
