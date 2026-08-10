package queue

import (
	"os"
	"path/filepath"
	"testing"
)

func loadMigrationBundleThroughPrefix(t *testing.T, maximum string) MigrationBundle {
	t.Helper()
	source, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	entries, err := os.ReadDir(source)
	if err != nil {
		t.Fatal(err)
	}
	foundMaximum := false
	for _, entry := range entries {
		name := entry.Name()
		if !migrationFileNamePattern.MatchString(name) || name[:3] > maximum {
			continue
		}
		if name[:3] == maximum {
			foundMaximum = true
		}
		raw, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, name), raw, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if !foundMaximum {
		t.Fatalf("migration prefix %s is unavailable", maximum)
	}
	return loadMigrationTestBundle(t, target)
}
