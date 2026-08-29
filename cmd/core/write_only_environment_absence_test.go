package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteOnlyRuntimeControlsAreAbsent(t *testing.T) {
	t.Parallel()
	root := coreRepositoryRoot(t)
	for _, name := range []string{
		"APP_ENV", "RETRIEVAL_LIMIT", "CONTEXT_CHAR_BUDGET",
		"WORKSPACE_MAX_FILES", "WORKSPACE_CONTEXT_BUDGET",
	} {
		for _, file := range []string{".env.example", "default.env", "docker-compose.yml"} {
			raw, err := os.ReadFile(filepath.Join(root, file))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), name) {
				t.Errorf("%s advertises removed write-only control %s", file, name)
			}
		}
	}
}
