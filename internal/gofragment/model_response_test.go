package gofragment

import (
	"strings"
	"testing"
)

func TestProjectFunctionModelResponseReturnsOneExactDeclarationSpan(t *testing.T) {
	raw := " \n```go\nfunc Recount(values []int) int { return len(values) }\n```\n"
	projection, err := ProjectFunctionModelResponseProjection(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := `func Recount(values []int) int { return len(values) }`
	if strings.Contains(projection.Source, "```") || projection.Source != want ||
		projection.Source != raw[projection.StartByte:projection.EndByte] {
		t.Fatalf("projected declaration=%+v", projection)
	}
	if _, err := ProjectFunctionModelResponseProjection(
		`func Renamed(values []int) int { return len(values) }`,
	); err != nil {
		t.Fatalf("projector assumed one required signature: %v", err)
	}
	for name, candidate := range map[string]string{
		"raw literal":    "func Fence() string { return \"```\" }",
		"fenced literal": "```go\nfunc Fence() string { return \"```\" }\n```",
	} {
		if _, err := ProjectFunctionModelResponseProjection(candidate); err != nil {
			t.Fatalf("%s rejected: %v", name, err)
		}
	}
}

func TestProjectFunctionModelResponseRejectsInvalidOrExpandedAuthority(t *testing.T) {
	for name, raw := range map[string]string{
		"empty":       "",
		"malformed":   `func Recount(values []int) int { return`,
		"extra":       "func Recount(values []int) int { return len(values) }\nfunc Audit(value int) int { return value }",
		"executable":  "var host = 1\nfunc Recount(values []int) int { return len(values) }",
		"prose":       "Here is the declaration:\nfunc Recount(values []int) int { return len(values) }",
		"wrong fence": "```javascript\nfunction recount(values) { return values.length; }\n```",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ProjectFunctionModelResponseProjection(raw); err == nil {
				t.Fatalf("projector accepted %s authority: %q", name, raw)
			}
		})
	}
}
