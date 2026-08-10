package worker

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRepositoryMutationWorkflowHasNoLegacyEvidenceWriter(t *testing.T) {
	t.Parallel()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve repository mutation source directory")
	}
	directory := filepath.Dir(current)
	patterns := []string{
		"v3_repository_change*.go",
		"v3_repository_mutation*.go",
	}
	for _, pattern := range patterns {
		files, err := filepath.Glob(filepath.Join(directory, pattern))
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			for _, forbidden := range []string{
				"recordExistingRepositoryGeneratedDiff",
				"recordExistingRepositoryApplyFailure",
				"evidence.KindGeneratedDiff",
			} {
				if strings.Contains(string(raw), forbidden) {
					t.Fatalf("repository mutation source %s retains forbidden evidence path %q", filepath.Base(file), forbidden)
				}
			}
		}
	}
}
