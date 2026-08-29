package architecture

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionGoHasNoAcceptanceGroundingReviewControlPlane(t *testing.T) {
	t.Parallel()

	root := filepath.Clean(filepath.Join("..", ".."))
	forbidden := []string{
		"AcceptanceGrounding",
		"acceptanceGrounding",
		"WorkApplicationAcceptanceGroundingReview",
		"ApplicationAcceptanceGrounding",
		"AcceptanceGroundingAccept",
		"AcceptanceGroundingRepair",
		"ensureDirectCodingAcceptanceGrounding",
		"AcceptanceObservationInventory",
		"InventoryTypeScriptAcceptanceObservations",
		"ResolveTypeScriptAcceptanceFailureEvidence",
		"ResolveTypeScriptAcceptanceObservationSite",
		"RewriteTypeScriptAcceptanceObservationQueryAlias",
		"RemoveTypeScriptAcceptanceObservationStatement",
		"application_acceptance_grounding_review",
		"CodingWorkloadReview",
		"coding_workload_review",
		"coding_workload_review_model",
	}
	const retiredEnvironmentKey = "OMNI_CODING_WORKLOAD_REVIEW_MODEL"
	retiredEnvironmentRegistrations := 0

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root {
				switch entry.Name() {
				case ".git", ".cache", "node_modules", "vendor":
					return filepath.SkipDir
				}
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		source := string(raw)
		for _, token := range forbidden {
			if strings.Contains(source, token) {
				t.Errorf("production source %s retains acceptance-review control %q", path, token)
			}
		}
		if strings.Contains(source, retiredEnvironmentKey) {
			if filepath.Clean(path) != filepath.Join(root, "internal", "modelconfig", "removed.go") {
				t.Errorf("production source %s retains retired model configuration %q", path, retiredEnvironmentKey)
			} else {
				retiredEnvironmentRegistrations += strings.Count(source, retiredEnvironmentKey)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if retiredEnvironmentRegistrations != 1 {
		t.Fatalf("retired acceptance-review environment registration count=%d want 1", retiredEnvironmentRegistrations)
	}
}
