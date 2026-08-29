package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionHasNoRepositoryReviewOrCorrectionCeremony(t *testing.T) {
	t.Parallel()
	root := filepath.Clean(filepath.Join("..", ".."))
	forbidden := []string{
		"RepositoryGroundedReview",
		"RepositoryGroundedCorrection",
		"WorkRepositoryGroundedIssueDetail",
		"WorkRepositoryGroundedIssueKind",
		"WorkRepositoryGroundedCorrection",
		"repository_grounded_review",
		"repository_grounded_issue_detail",
		"repository_grounded_issue_kind",
		"repository_grounded_correction",
		"OMNI_REPOSITORY_GROUNDED_REVIEW_MODEL",
		"OMNI_REPOSITORY_GROUNDED_CORRECTION_MODEL",
	}
	for _, relative := range []string{"cmd", "internal"} {
		walkProductionSource(t, filepath.Join(root, relative), func(path, source string) {
			for _, token := range forbidden {
				if filepath.Base(path) == "removed.go" && strings.HasPrefix(token, "OMNI_") {
					continue
				}
				if strings.Contains(source, token) {
					t.Errorf("production source %s retains repository ceremony %q", path, token)
				}
			}
		})
	}
	for _, relative := range []string{"default.env", ".env.example"} {
		raw, err := os.ReadFile(filepath.Join(root, relative))
		if err != nil {
			t.Fatal(err)
		}
		for _, token := range forbidden {
			if strings.Contains(string(raw), token) {
				t.Errorf("configuration template %s retains repository ceremony %q", relative, token)
			}
		}
	}
}
