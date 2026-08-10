package queue

import (
	"os"
	"strings"
	"testing"
)

func TestSchemaInstallHasNoPostBundleBackfillPath(t *testing.T) {
	for _, name := range []string{"repository.go", "repository_migrate_fresh.go"} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		source := string(raw)
		for _, forbidden := range []string{
			"BackfillMemoryCategories", "BackfillScrumBoardOrder",
		} {
			if strings.Contains(source, forbidden) {
				t.Fatalf("%s retains post-bundle mutation %q", name, forbidden)
			}
		}
	}

	for _, name := range []string{"repository_memory.go", "projects.go"} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "func (r *Repository) Backfill") {
			t.Fatalf("%s retains a dead exported backfill authority", name)
		}
	}
}
