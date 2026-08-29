package worker

import (
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func loadWorkerDatabaseSetup(t testing.TB) queue.DatabaseSetup {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "..", "database", "setup.sql"))
	if err != nil {
		t.Fatal(err)
	}
	setup, err := queue.LoadDatabaseSetup(path)
	if err != nil {
		t.Fatal(err)
	}
	return setup
}
