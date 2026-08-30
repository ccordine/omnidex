package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionHasNoRepositoryRequirementCoverageLoop(t *testing.T) {
	t.Parallel()
	root := filepath.Clean(filepath.Join("..", ".."))
	retiredFile := filepath.Join(
		root,
		"internal",
		"assemblyline",
		"repository_requirement_leaves.go",
	)
	if _, err := os.Stat(retiredFile); !os.IsNotExist(err) {
		t.Fatalf("retired repository requirement leaf source still exists: %v", err)
	}
	for _, relative := range []string{
		"internal/assemblyline",
		"internal/queue",
		"internal/worker",
	} {
		walkProductionSource(t, filepath.Join(root, relative), func(path, source string) {
			for _, forbidden := range []string{
				"WorkRepositoryRequirementCoverage",
				"RepositoryRequirementLeafInput",
				"NewRepositoryRequirementCoverageJob",
				"NewRepositoryRequirementJob",
				"BuildRepositoryRequirementCoveragePrompt",
				"BuildRepositoryRequirementPrompt",
				"DecodeRepositoryRequirementCoverageLeaf",
				"DecodeRepositoryRequirementLeaf",
				"RepositoryRequirementRemains",
				"RepositoryNoUncoveredRequirement",
				`"repository_requirement_coverage"`,
				`"repository_requirement"`,
			} {
				if strings.Contains(source, forbidden) {
					t.Errorf("production source %s retains repository coverage-loop authority %q", path, forbidden)
				}
			}
		})
	}
	schema, err := os.ReadFile(filepath.Join(root, "database", "setup.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"'repository_requirement_coverage'",
		"WHEN 'repository_requirement' THEN",
	} {
		if strings.Contains(string(schema), forbidden) {
			t.Errorf("database schema retains repository coverage-loop authority %q", forbidden)
		}
	}
}
