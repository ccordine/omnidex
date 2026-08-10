package queue

import (
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/version"
)

func loadCheckedMigrationBundle(t testing.TB) MigrationBundle {
	t.Helper()
	directory, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := LoadMigrationBundle(directory, version.MigrationsSHA256)
	if err != nil {
		t.Fatalf("load checked migration bundle: %v", err)
	}
	return bundle
}
