package worker

import (
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestGoIdentifierTrialsDoNotRecursivelyEnumerateOtherDefects(t *testing.T) {
	for _, fixture := range []struct{ name, signature, body string }{
		{"numeric expression", "func Transform(amount int) int", "return missing.Unknown(amount)"},
		{"text expression", "func Render(label string) string", "value := label\nreturn missing.Other(value)"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			input := assemblyline.FragmentGenerationInput{Language: "go", Dialect: "Go", Signature: fixture.signature, Behavior: fixture.name}
			_, err := validateDirectCodingGoFragment(input, fixture.body)
			var defect *assemblyline.SourceBodyDefect
			if !errors.As(err, &defect) {
				t.Fatalf("validation error=%v; want exact unresolved identifier", err)
			}
			start, end, err := defect.MutableRange(fixture.body)
			if err != nil {
				t.Fatal(err)
			}
			if got := fixture.body[start:end]; got != "missing" {
				t.Fatalf("first unresolved span=%q; want missing", got)
			}
		})
	}
}
