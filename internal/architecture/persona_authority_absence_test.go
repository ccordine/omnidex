package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPersonaAndFilesystemSkillAuthorityAreAbsent(t *testing.T) {
	t.Parallel()

	root := filepath.Join("..", "..")
	for _, name := range []string{"skills", "internal/specialist"} {
		err := filepath.WalkDir(filepath.Join(root, name), func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if !entry.IsDir() {
				t.Fatalf("rejected static authority file remains: %s", path)
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			t.Fatalf("inspect rejected path %s: %v", name, err)
		}
	}
	checks := map[string][]string{
		"cmd/cli/chat_command.go": {
			"model-plan", "model-analyze", "model-response", "model-search", "model-tagger", "model-verify", "model-memory",
		},
		"cmd/cli/coding_run_command.go": {"model_execute", "model-source", "model-fragment"},
		"internal/worker/engine.go": {
			"SkillsRoot", "bootstrapRegistry", "v3Registry", "LoadRegistry", "Specialist map",
		},
		"internal/worker/engine_options.go": {"modelRoles", "models.plan is required"},
		"internal/worker/step_runner.go": {
			"specialist_role", "ForPipelineAction", "DetailLines", "specialist=",
		},
		"internal/worker/model_override.go": {
			"v3SpecialistModel", "skillPreferredModel", "specialistRoleForJob",
		},
		"Dockerfile": {"/src/skills"},
	}
	for name, forbidden := range checks {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, token := range forbidden {
			if strings.Contains(string(raw), token) {
				t.Fatalf("rejected authority token %q remains in %s", token, name)
			}
		}
	}
}
