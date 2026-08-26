package assemblyline

import (
	"strings"
	"testing"
)

func TestGoFragmentModificationCarriesOnePathBlindDeclaration(t *testing.T) {
	t.Parallel()
	job, err := NewFragmentModificationJob(FragmentModificationInput{
		Language: "go", Dialect: "Go 1.24", Signature: "func Value() int",
		CurrentDeclaration: "func Value() int { return 1 }",
		RequirementQuote:   "return two", Capabilities: []string{"func Helper() int"},
		PermittedSymbols: []string{"Helper"},
	})
	if err != nil {
		t.Fatal(err)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if schema != nil || !strings.Contains(prompt, "CURRENT_DECLARATION") || !strings.Contains(prompt, "return two") {
		t.Fatalf("prompt=%q schema=%#v", prompt, schema)
	}
	if !strings.Contains(prompt, "SOURCE_DIALECT:\nGo 1.24") {
		t.Fatalf("prompt omits source dialect: %q", prompt)
	}
	for _, forbidden := range []string{"workspace tree", "file path", "shell command"} {
		if strings.Contains(strings.ToLower(prompt), forbidden) {
			t.Fatalf("prompt exposes forbidden concept %q", forbidden)
		}
	}
}
