package queue

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const workspaceMutationCutoverMigration = "159_workspace_mutation_journal_cutover.sql"

func TestWorkspaceMutationCutoverMigrationDefinesGenericVerifiedAuthority(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join("..", "..", "migrations", workspaceMutationCutoverMigration))
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	required := []string{
		"LOCK TABLE repository_mutation_operations, repository_mutation_files",
		"status <> 'applied'",
		"workspace mutation cutover rejects unresolved legacy repository mutation",
		"ALTER TABLE repository_mutation_operations RENAME TO retired_repository_mutation_operations",
		"ALTER TABLE repository_mutation_files RENAME TO retired_repository_mutation_files",
		"CREATE TABLE workspace_mutation_operations",
		"CREATE TABLE workspace_mutation_files",
		"^workspace_mutation_[0-9a-f]{64}$",
		"^workspace_[0-9a-f]{64}$",
		"^workspace_state_[0-9a-f]{64}$",
		"source_repository_snapshot_id TEXT",
		"verified_repository_snapshot_id TEXT",
		"octet_length(patch) BETWEEN 1 AND 33554432",
		"ordinal BETWEEN 0 AND 31",
		"workspace mutation must be sealed with 1-32 exact files",
		"'prepared','applying','applied','verifying','verified','verification_failed','indeterminate'",
		"indeterminate_phase IN ('apply','verification')",
		"workspace mutation verification receipt",
		"workspace_verification_receipt",
		"idx_workspace_mutations_unresolved_generation",
		"idx_workspace_mutations_unresolved_workspace",
		"prevent_retired_repository_mutation_change",
		"prevent_workspace_mutation_evidence_change",
	}
	for _, fragment := range required {
		if !strings.Contains(source, fragment) {
			t.Errorf("migration %s is missing %q", workspaceMutationCutoverMigration, fragment)
		}
	}
	for _, forbidden := range []string{
		"CREATE VIEW repository_mutation_operations",
		"CREATE VIEW repository_mutation_files",
		"CREATE TABLE repository_mutation_operations",
		"CREATE TABLE repository_mutation_files",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("migration %s retains forbidden legacy authority %q", workspaceMutationCutoverMigration, forbidden)
		}
	}
}
