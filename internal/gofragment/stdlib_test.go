package gofragment

import (
	"strings"
	"testing"
)

func TestGoStandardLibraryReferences(t *testing.T) {
	for _, fixture := range []struct{ name, signature, body string }{
		{"numeric magnitude", "func Magnitude(value float64) float64", "return math.Abs(value)"},
		{"binary encoding", "func Encode(value []byte) string", "return hex.EncodeToString(value)"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			if _, err := ParseNewFunctionBody(fixture.signature, nil, fixture.body); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestGoStandardLibraryRejectsUnknownAndAmbiguousMembers(t *testing.T) {
	for _, body := range []string{"return math.Invented(value)", "return rand.Read(value)"} {
		if _, err := ParseNewFunctionBody("func Apply(value []byte) int", nil, body); err == nil {
			t.Fatalf("accepted unresolved standard-library reference %q", body)
		}
	}
}

func TestGoStandardLibraryDoesNotGrantUnqualifiedNames(t *testing.T) {
	_, err := ParseNewFunctionBody("func Magnitude(value float64) float64", nil, "_ = math.Abs(value)\nreturn Abs(value)")
	if err == nil || !strings.Contains(err.Error(), `"Abs"`) {
		t.Fatalf("unqualified import member error=%v", err)
	}
}

func TestGoStandardLibraryPreservesLocalNamespaceAuthority(t *testing.T) {
	source := "func Value(math struct { Abs float64 }) float64 { return math.Abs }"
	paths, err := StandardLibraryImports(source, nil)
	if err != nil || len(paths) != 0 {
		t.Fatalf("local parameter imports=%v error=%v", paths, err)
	}
}

func TestGoStandardLibraryRejectsConflictingPackageQualifiers(t *testing.T) {
	source := "func Value() { _ = rand.Intn(1); _ = rand.Reader }"
	if _, err := StandardLibraryImports(source, nil); err == nil {
		t.Fatal("one qualifier was allowed to import two different packages")
	}
}
