package queue

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTransactionalStepAttemptFenceHasOneDatabaseAuthorityPath(t *testing.T) {
	authorizer, err := os.ReadFile(filepath.Join(".", "step_attempt_authorize.go"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(authorizer)
	if strings.Contains(source, "requireActiveStepAttemptTx(ctx, tx, authority)") ||
		!strings.Contains(source, "callStepAttemptFenceTransaction") {
		t.Fatal("exported transactional authorizer retains a raw-table authority path")
	}

	setup, err := os.ReadFile(filepath.Join(
		"..", "..", "database", "setup.sql",
	))
	if err != nil {
		t.Fatal(err)
	}
	setupSource := string(setup)
	for _, required := range []string{
		"SECURITY DEFINER", "SET search_path TO pg_catalog, %I",
		"REVOKE ALL ON FUNCTION", "FROM PUBLIC", "FOR UPDATE",
	} {
		if !strings.Contains(setupSource, required) {
			t.Fatalf("transactional fence setup lacks %q", required)
		}
	}
}

func TestNewProductionFilesRemainFocused(t *testing.T) {
	files := []string{
		"database_setup.go", "database_setup_lock.go", "database_reset.go",
		"step_attempt_database_fence.go", "step_attempt_authorizer_provision.go",
		"step_attempt_authorizer_role.go",
	}
	for _, name := range files {
		raw, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatal(err)
		}
		if lines := strings.Count(string(raw), "\n") + 1; lines > 300 {
			t.Fatalf("%s has %d lines, want at most 300", name, lines)
		}
	}
}
