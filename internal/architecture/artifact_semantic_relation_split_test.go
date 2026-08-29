package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionHasNoCompoundKnownArtifactTruthStation(t *testing.T) {
	t.Parallel()
	root := filepath.Clean(filepath.Join("..", ".."))
	for _, relative := range []string{
		"internal/assemblyline/known_artifact_truth.go",
		"internal/worker/v3_known_artifact_truth.go",
		"internal/worker/v3_known_artifact_truth_validation.go",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(relative))); !os.IsNotExist(err) {
			t.Errorf("retired compound artifact-truth source %s still exists: %v", relative, err)
		}
	}
	for _, relative := range []string{
		"internal/assemblyline",
		"internal/config",
		"internal/modelconfig",
		"internal/queue",
		"internal/station",
		"internal/worker",
	} {
		walkProductionSource(t, filepath.Join(root, relative), func(path, source string) {
			for _, forbidden := range []string{
				"KnownArtifactTruth", "WorkKnownArtifactTruth",
				"CodingKnownArtifactTruth", `"known_artifact_truth"`,
				`"coding_known_artifact_truth"`,
			} {
				if strings.Contains(source, forbidden) {
					t.Errorf("production source %s retains compound artifact truth authority %q", path, forbidden)
				}
			}
		})
	}
}

func TestArtifactSemanticPromptsCannotAnswerTheOtherRelation(t *testing.T) {
	t.Parallel()
	root := filepath.Clean(filepath.Join("..", "assemblyline"))
	checks := []struct {
		file      string
		required  string
		forbidden string
	}{
		{
			file:      "repository_artifact_absence.go",
			required:  "repository_artifact_must_be_absent",
			forbidden: "one_new_complete_plain_text_artifact_required",
		},
		{
			file:      "plain_text_artifact_creation.go",
			required:  "one_new_complete_plain_text_artifact_required",
			forbidden: "repository_artifact_must_be_absent",
		},
	}
	for _, check := range checks {
		raw, err := os.ReadFile(filepath.Join(root, check.file))
		if err != nil {
			t.Fatal(err)
		}
		source := string(raw)
		if !strings.Contains(source, check.required) {
			t.Errorf("%s omitted its raw semantic relation %q", check.file, check.required)
		}
		if strings.Contains(source, check.forbidden) {
			t.Errorf("%s contains the other station's relation %q", check.file, check.forbidden)
		}
	}
}
