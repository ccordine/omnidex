package gofragment

import "testing"

func TestProjectFunctionModelResponseRequiresOneExactDeclaration(t *testing.T) {
	t.Parallel()
	raw := "func Recount(values []int) int { return len(values) }"
	projection, err := ProjectFunctionModelResponseProjection(raw)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Source != raw || projection.StartByte != 0 ||
		projection.EndByte != len(raw) {
		t.Fatalf("projected declaration=%+v", projection)
	}
	if _, err := ProjectFunctionModelResponseProjection(
		`func Renamed(values []int) int { return len(values) }`,
	); err != nil {
		t.Fatalf("projector assumed one required signature: %v", err)
	}
	if _, err := ProjectFunctionModelResponseProjection(
		"func Fence() string { return \"```\" }",
	); err != nil {
		t.Fatalf("literal fence text inside declaration was rejected: %v", err)
	}
}

func TestProjectFunctionModelResponseRejectsInvalidOrOuterAuthority(t *testing.T) {
	t.Parallel()
	source := `func Recount(values []int) int { return len(values) }`
	for name, raw := range map[string]string{
		"empty":            "",
		"leading space":    " " + source,
		"trailing newline": source + "\n",
		"malformed":        `func Recount(values []int) int { return`,
		"extra":            source + "\nfunc Audit(value int) int { return value }",
		"executable":       "var host = 1\n" + source,
		"prose":            "Here is the declaration:\n" + source,
		"comment":          "// generated source\n" + source,
		"Go fence":         "```go\n" + source + "\n```",
		"JSON fence":       "```json\n{\"source\":\"value\"}\n```",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ProjectFunctionModelResponseProjection(raw); err == nil {
				t.Fatalf("projector accepted %s authority: %q", name, raw)
			}
		})
	}
}
