package gofragment

import (
	"strings"
	"testing"
)

func TestGoResponseSelectsRequestedBodyFromAdditionalDeclarations(t *testing.T) {
	for _, fixture := range []struct{ name, signature, response, wanted string }{
		{
			"numeric behavior", "func Double(value int) int",
			"```go\npackage example\nfunc Unrelated() int { return 99 }\nfunc Double(value int) int { return value * 2 }\n```\n```text\nExample output\n```",
			"return value * 2",
		},
		{
			"text behavior", "func Enclose(value string) string",
			"```go\npackage example\nimport \"fmt\"\ntype Unrelated struct{}\nfunc Enclose(value string) string { return \"[\" + value + \"]\" }\nfunc Extra() { fmt.Println(\"unused\") }\n```",
			"return \"[\" + value + \"]\"",
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			got, err := ParseNewFunctionBody(fixture.signature, nil, fixture.response)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(got, fixture.wanted) || strings.Contains(got, "Unrelated") || strings.Contains(got, "Extra") || strings.Contains(got, "import") {
				t.Fatalf("unexpected accepted source: %s", got)
			}
		})
	}
}

func TestGoResponseCannotGrantAnExtraHelper(t *testing.T) {
	_, err := ParseNewFunctionBody("func Value() int", nil, "package example\nfunc Value() int { return invented() }\nfunc invented() int { return 5 }")
	if err == nil || !strings.Contains(err.Error(), "invented") {
		t.Fatalf("extra helper gained capability authority: %v", err)
	}
}

func TestGoResponseProcessesBodyCandidatesInOrder(t *testing.T) {
	for _, fixture := range []struct{ name, signature, invalid, first, later string }{
		{"numeric", "func Double(value int) int", "return missing(value)", "return value * 2", "return value + value"},
		{"text", "func Enclose(value string) string", "return missing(value)", `return "[" + value + "]"`, `return fmt.Sprintf("[%s]", value)`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			for _, wrapped := range []bool{false, true} {
				response := ""
				for _, body := range []string{fixture.invalid, fixture.first, fixture.later} {
					if wrapped {
						body = fixture.signature + " { " + body + " }"
					}
					response += "```go\n" + body + "\n```\n"
				}
				got, err := ParseNewFunctionBody(fixture.signature, nil, response)
				if err != nil {
					t.Fatal(err)
				}
				want, err := ParseNewFunctionBody(fixture.signature, nil, fixture.first)
				if err != nil || got != want {
					t.Fatalf("selected body = %s, want first valid body %s: %v", got, want, err)
				}
			}
		})
	}
}
