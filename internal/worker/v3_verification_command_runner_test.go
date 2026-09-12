package worker

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/queue"
)

func TestVerificationEvidencePersistenceSurvivesCommandCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	persistence, stop := directCodingVerificationEvidenceContext(parent)
	defer stop()
	if err := persistence.Err(); err != nil {
		t.Fatalf("bounded evidence persistence inherited command cancellation: %v", err)
	}
}

func TestHostCleanupExecutionSurvivesRuntimeCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	cleanup, stopCleanup := directCodingVerificationCommandContext(
		parent, queue.VerificationHostCleanup, time.Second,
	)
	defer stopCleanup()
	if err := cleanup.Err(); err != nil {
		t.Fatalf("host cleanup inherited runtime cancellation: %v", err)
	}
	ordinary, stopOrdinary := directCodingVerificationCommandContext(
		parent, queue.VerificationHostFinal, time.Second,
	)
	defer stopOrdinary()
	if err := ordinary.Err(); err == nil {
		t.Fatal("ordinary verification unexpectedly escaped runtime cancellation")
	}
}

func TestVerificationProcessEnvironmentExcludesAmbientSecrets(t *testing.T) {
	t.Setenv("OMNIDEX_SENTINEL_SECRET", "must-not-cross-command-boundary")
	environment, err := directCodingVerificationProcessEnvironment([]string{
		"CI=1", "NPM_CONFIG_USERCONFIG=/dev/null",
	})
	if err != nil {
		t.Fatalf("construct verification environment: %v", err)
	}
	if !sort.StringsAreSorted(environment) {
		t.Fatalf("verification environment is not canonical: %v", environment)
	}
	joined := strings.Join(environment, "\n")
	if strings.Contains(joined, "OMNIDEX_SENTINEL_SECRET") ||
		strings.Contains(joined, "must-not-cross-command-boundary") {
		t.Fatalf("ambient secret crossed verification boundary: %v", environment)
	}
	for _, expected := range []string{
		"CI=1", "LANG=C.UTF-8", "LC_ALL=C.UTF-8", "NPM_CONFIG_USERCONFIG=/dev/null",
		"TMPDIR=/tmp",
	} {
		if !containsExactString(environment, expected) {
			t.Fatalf("verification environment omits %q: %v", expected, environment)
		}
	}
	if errEnvironment, err := directCodingVerificationProcessEnvironment(
		[]string{"DATABASE_URL=forbidden"},
	); err == nil || errEnvironment != nil {
		t.Fatalf("secret-bearing environment name unexpectedly accepted: %v", errEnvironment)
	}
}

func TestGeneratedHostCleanupTargetsOnlyAttemptCreatedRoots(t *testing.T) {
	t.Run("fresh workspace", func(t *testing.T) {
		root := t.TempDir()
		initial, err := snapshotDirectCodingGeneratedHostPaths(root)
		if err != nil {
			t.Fatalf("snapshot fresh generated roots: %v", err)
		}
		created := directCodingGeneratedHostPathsCreatedByAttempt(initial)
		if !sameExactStrings(created, directCodingTypeScriptGeneratedHostPaths) {
			t.Fatalf("fresh cleanup targets=%v; want %v", created, directCodingTypeScriptGeneratedHostPaths)
		}
		for _, relative := range created {
			if err := os.Mkdir(filepath.Join(root, relative), 0o700); err != nil {
				t.Fatalf("create generated root %s: %v", relative, err)
			}
			if err := os.RemoveAll(filepath.Join(root, relative)); err != nil {
				t.Fatalf("remove attempt-created root %s: %v", relative, err)
			}
		}
		if err := validateDirectCodingGeneratedHostPathRetention(root, initial); err != nil {
			t.Fatalf("validate fresh cleanup: %v", err)
		}
	})

	t.Run("pre-existing roots", func(t *testing.T) {
		root := t.TempDir()
		for _, relative := range []string{"node_modules", "dist"} {
			if err := os.Mkdir(filepath.Join(root, relative), 0o700); err != nil {
				t.Fatalf("create pre-existing root %s: %v", relative, err)
			}
			if err := os.WriteFile(
				filepath.Join(root, relative, "retained.txt"), []byte("retained"), 0o600,
			); err != nil {
				t.Fatalf("write pre-existing marker %s: %v", relative, err)
			}
		}
		if err := requireAbsentDirectCodingGeneratedHostPaths(root); err == nil {
			t.Fatal("pre-existing generated roots unexpectedly received mutation authority")
		}
		for _, relative := range []string{"node_modules", "dist"} {
			content, err := os.ReadFile(filepath.Join(root, relative, "retained.txt"))
			if err != nil || string(content) != "retained" {
				t.Fatalf("pre-existing marker %s was removed: %q, %v", relative, content, err)
			}
		}
	})
}

func containsExactString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func sameExactStrings(first []string, second []string) bool {
	if len(first) != len(second) {
		return false
	}
	for index := range first {
		if first[index] != second[index] {
			return false
		}
	}
	return true
}
