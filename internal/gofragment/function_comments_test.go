package gofragment

import (
	"errors"
	"strings"
	"testing"
)

func TestGoSourceBodyAcceptsOrdinaryComments(t *testing.T) {
	for _, fixture := range []struct {
		name, signature, body, bare string
	}{
		{
			"temperature conversion", "func Kelvin(celsius float64) float64",
			"// Convert the temperature.\nreturn celsius + 273.15 // Absolute scale.",
			"return celsius + 273.15",
		},
		{
			"text enclosure", "func Enclose(text string) string",
			"/* Enclose the supplied text. */\nreturn \"[\" + text + \"]\"",
			"return \"[\" + text + \"]\"",
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			want, err := ParseNewFunctionBody(fixture.signature, nil, fixture.bare)
			if err != nil {
				t.Fatal(err)
			}
			got, err := ParseNewFunctionBody(fixture.signature, nil, fixture.body)
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("ordinary comments changed assembled source: %q; want %q", got, want)
			}
		})
	}
}

func TestGoSourceBodyRejectsCompilerAndLineDirectives(t *testing.T) {
	for _, directive := range []string{
		"//go:noinline", "//go:build ignore", "//line injected.go:100", "/*line injected.go:100*/",
	} {
		t.Run(directive, func(t *testing.T) {
			_, err := ParseNewFunctionBody("func Value() int", nil, directive+"\nreturn 1")
			if err == nil || !strings.Contains(err.Error(), "directives are forbidden") {
				t.Fatalf("directive validation error=%v", err)
			}
		})
	}
}

func TestGoSourceBodyAllowsDirectiveTextInsideString(t *testing.T) {
	_, err := ParseNewFunctionBody("func Label() string", nil, `return "//go:noinline //line label:1 /*line label:2*/"`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGoSourceBodyCommentsPreserveExactFailureSpan(t *testing.T) {
	body := "// A local explanation.\nreturn missing"
	_, err := ParseNewFunctionBody("func Value() int", nil, body)
	var violation *BodySpanViolation
	if !errors.As(err, &violation) {
		t.Fatalf("scope error=%v; want located body violation", err)
	}
	if got := body[violation.StartByte:violation.EndByte]; got != "missing" {
		t.Fatalf("failed span=%q; want missing", got)
	}
}
