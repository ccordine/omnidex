package assemblyline

import (
	"strings"
	"testing"
)

func TestGoFragmentGenerationIsOnePathBlindDeclaration(t *testing.T) {
	t.Parallel()
	job, err := NewFragmentGenerationJob(FragmentGenerationInput{
		Language: "go", Dialect: "Go 1.24", Signature: "func Added() int", Behavior: "return two",
	})
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "func Added() int") || !strings.Contains(prompt, "return two") {
		t.Fatalf("prompt=%q", prompt)
	}
	for _, forbidden := range []string{"filename", "target path", "create_file", "delete_file", "write_file", "shell command"} {
		if strings.Contains(strings.ToLower(prompt), forbidden) {
			t.Fatalf("Go generation prompt exposed forbidden authority %q: %s", forbidden, prompt)
		}
	}
}
