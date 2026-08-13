package queue

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryMutationFileTransitionMigrationFreezesExactPresenceAuthority(t *testing.T) {
	t.Parallel()
	path := filepath.Join(
		"..", "..", "migrations", "082_repository_mutation_file_state_transitions.sql",
	)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	source := string(content)
	for _, required := range []string{
		"source_present",
		"source_mode",
		"expected_present",
		"expected_mode",
		"validate_repository_mutation_file_source",
		"repository mutation source absence disagrees with immutable source snapshot",
		"desired_graph_",
		"unresolved repository mutations must be recovered before file-state migration",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("file-state transition migration lacks %q", required)
		}
	}
	for _, forbidden := range []string{
		"mutation_kind",
		"operation_kind",
		"create_file",
		"delete_file",
		"rename_file",
		"move_file",
	} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("file-state transition migration exposes forbidden operation authority %q", forbidden)
		}
	}
}
