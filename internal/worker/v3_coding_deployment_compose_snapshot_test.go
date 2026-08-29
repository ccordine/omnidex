package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/operation"
)

func TestRollbackComposeSnapshotDriftCannotReachCommandExecutor(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name  string
		drift func(*testing.T, string)
	}{
		{
			name: "mutated bytes",
			drift: func(t *testing.T, root string) {
				path := filepath.Join(root, directCodingDeploymentComposePath)
				if err := os.Chmod(path, 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("services:\n  changed:\n    image: invalid\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(path, 0o444); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "deleted source",
			drift: func(t *testing.T, root string) {
				if err := os.Chmod(root, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(filepath.Join(root, directCodingDeploymentComposePath)); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(root, 0o555); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "symlink source",
			drift: func(t *testing.T, root string) {
				target := filepath.Join(filepath.Dir(root), "foreign-compose.yml")
				if err := os.WriteFile(target, []byte("services: {}\n"), 0o444); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(root, 0o755); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(root, directCodingDeploymentComposePath)
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(root, 0o555); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "replaced root",
			drift: func(t *testing.T, root string) {
				if err := os.Rename(root, root+".replaced"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(root, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(
					filepath.Join(root, directCodingDeploymentComposePath),
					[]byte("services:\n  replacement:\n    image: invalid\n"), 0o444,
				); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(root, 0o555); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			authority := deploymentComposeSnapshotAuthorityFixture(t)
			testCase.drift(t, authority.Root)
			executions := 0
			_, err := executeDirectCodingComposeSnapshotBoundCommand(
				authority, authority.Root,
				func(string) (operation.Result, error) {
					executions++
					return operation.Result{}, nil
				},
			)
			if err == nil || !errors.Is(err, errDirectCodingDeploymentSnapshotDrift) {
				t.Fatalf("rollback Compose snapshot drift error=%v", err)
			}
			if executions != 0 {
				t.Fatalf("rollback snapshot drift reached %d command executors", executions)
			}
		})
	}
}

func TestRollbackComposeSnapshotExecutesOnceAfterExactVerification(t *testing.T) {
	t.Parallel()
	authority := deploymentComposeSnapshotAuthorityFixture(t)
	executions := 0
	_, err := executeDirectCodingComposeSnapshotBoundCommand(
		authority, authority.Root,
		func(root string) (operation.Result, error) {
			executions++
			if root != authority.Root {
				t.Fatalf("rollback execution root=%q want %q", root, authority.Root)
			}
			return operation.Result{}, nil
		},
	)
	if err != nil || executions != 1 {
		t.Fatalf("exact rollback snapshot executions=%d error=%v", executions, err)
	}
}

func TestRollbackComposeSnapshotVerificationBracketsDurableJournal(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("v3_coding_deployment_rollback.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	preJournal := strings.Index(text, "rollbackSnapshot.ExecutionRoot()")
	qualification := strings.Index(text, "qualifyDirectCodingDeploymentRuntime(")
	journal := strings.Index(text, "BeginGeneratedWorkloadDeploymentRollbackAttempt(")
	postJournal := strings.Index(text, "executeDirectCodingComposeSnapshotBoundCommand(")
	executor := strings.Index(text, "executeDirectCodingDeploymentCommand(")
	if preJournal < 0 || qualification <= preJournal || journal <= qualification ||
		postJournal <= journal || executor <= postJournal {
		t.Fatalf(
			"rollback snapshot/runtime/journal/spawn order=%d/%d/%d/%d/%d",
			preJournal, qualification, journal, postJournal, executor,
		)
	}
	for file, binding := range map[string]string{
		"v3_coding_deployment_preparation.go":                 "rollbackSnapshot: rollbackSnapshot",
		"v3_coding_deployment_recovery_reconstruct.go":        "rollbackSnapshot: rollbackSnapshot",
		"v3_coding_deployment_recovery_rollback_execution.go": "runtime.prepared.rollbackSnapshot = composeSnapshot",
	} {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(source), binding) {
			t.Fatalf("rollback path %s omits persisted Compose snapshot binding", file)
		}
	}
}

func deploymentComposeSnapshotAuthorityFixture(
	t *testing.T,
) directCodingDeploymentComposeSnapshotAuthority {
	t.Helper()
	sourceRoot := t.TempDir()
	t.Cleanup(func() {
		_ = filepath.WalkDir(sourceRoot, func(path string, entry os.DirEntry, _ error) error {
			if entry != nil {
				_ = os.Chmod(path, 0o700)
			}
			return nil
		})
	})
	workspaceSHA256 := strings.Repeat("a", 64)
	content := []byte("services:\n  app:\n    image: example.invalid/app@sha256:" + strings.Repeat("b", 64) + "\n")
	digest := sha256.Sum256(content)
	composeSHA256 := hex.EncodeToString(digest[:])
	root := filepath.Join(
		sourceRoot, ".omni", directCodingDeploymentSnapshotDirectory, workspaceSHA256,
	)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, directCodingDeploymentComposePath), content, 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o555); err != nil {
		t.Fatal(err)
	}
	authority, err := directCodingOpenPersistedDeploymentComposeSnapshot(
		sourceRoot, workspaceSHA256, composeSHA256,
	)
	if err != nil {
		t.Fatal(err)
	}
	return authority
}
