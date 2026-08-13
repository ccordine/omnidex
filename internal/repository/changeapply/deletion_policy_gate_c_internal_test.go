package changeapply

import (
	"strings"
	"testing"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

func TestDeletionCandidateRejectsVendorAndProtectedTargetsBeforeInference(t *testing.T) {
	t.Parallel()
	for name, path := range map[string]string{
		"vendor":    "vendor/example.test/dependency/obsolete.go",
		"protected": ".omni/obsolete.go",
	} {
		t.Run(name, func(t *testing.T) {
			fileID := "file_" + strings.Repeat(name[:1], 64)
			snapshot := repositoryfacts.Snapshot{Files: []repositoryfacts.File{{
				ID: fileID, Path: path, Kind: repositoryfacts.EntryRegular,
				Language: "go", SHA256: strings.Repeat("a", 64), Size: 1, Mode: 0o644,
			}}}
			_, err := exactDeletionCandidateFile(snapshot, fileID)
			if err == nil || (!strings.Contains(err.Error(), "vendored") &&
				!strings.Contains(err.Error(), "protected")) {
				t.Fatalf("excluded deletion candidate error=%v", err)
			}
		})
	}
}
