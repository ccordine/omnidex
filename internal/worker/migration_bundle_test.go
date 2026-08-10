package worker

import (
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/version"
)

func loadWorkerTestMigrationBundle(t testing.TB) queue.MigrationBundle {
	t.Helper()
	directory, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := queue.LoadMigrationBundle(directory, version.MigrationsSHA256)
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}
