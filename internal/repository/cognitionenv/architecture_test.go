package cognitionenv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionRepositoryCognitionHasNoBenchmarkMutationOrFilesystemDependency(t *testing.T) {
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(raw)
		for _, forbidden := range []string{
			"internal/labyrinth", "internal/cognitiongauntlet", "repository/changeapply",
			"internal/queue", "internal/cognitionstore", `"os"`, `"os/exec"`,
			"repository.write", "repository.mutate", "repository.finish",
		} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("production repository cognition file %s contains forbidden dependency %q", path, forbidden)
			}
		}
	}
}
