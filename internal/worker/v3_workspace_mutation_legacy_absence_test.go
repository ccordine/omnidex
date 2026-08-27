package worker

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestProductionOmitsLegacyRepositoryMutationJournalPath(t *testing.T) {
	t.Parallel()
	repositoryRoot := workspaceMutationCutoverRepositoryRoot(t)
	forbidden := map[string]string{
		"RepositoryMutationCommand":            "legacy queue command type",
		"RepositoryMutationFile":               "legacy queue file type",
		"RepositoryMutationClassifier":         "legacy queue classifier type",
		"RepositoryMutationState":              "legacy queue observation type",
		"ApplyRepositoryMutation(":             "legacy queue apply API",
		"UnresolvedRepositoryMutation(":        "legacy queue recovery API",
		"repository_mutation_operations":       "legacy mutation operation table",
		"repository_mutation_files":            "legacy mutation file table",
		"repository_mutation_recovery_started": "legacy recovery-start event",
		"repository_mutation_recovered":        "legacy recovery-complete event",
	}
	err := filepath.WalkDir(repositoryRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path == filepath.Join(repositoryRoot, "internal", "queue") ||
				entry.Name() == ".git" || entry.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for token, label := range forbidden {
			if strings.Contains(string(raw), token) {
				t.Errorf("production source %s retains %s %q", path, label, token)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceMutationRecoveryReturnsBeforeWorkspaceClassification(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("v3_coding_runtime.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	recovery := strings.Index(source, "r.reconcileCurrentWorkspaceMutation(scope.Root, request)")
	classification := strings.Index(source, "directCodingWorkspaceHasImplementation(scope.Root, nil)")
	if recovery < 0 || classification < 0 || recovery >= classification {
		t.Fatalf(
			"workspace recovery must precede classification: recovery=%d classification=%d",
			recovery, classification,
		)
	}
	guard := source[recovery:classification]
	if !strings.Contains(guard, "handled || err != nil") ||
		!strings.Contains(guard, "return summary, err") {
		t.Fatal("workspace recovery lacks an early return before workspace classification")
	}
}

func workspaceMutationCutoverRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve workspace mutation cutover test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
}
