package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gryph/omnidex/internal/queue"
)

func loadCoreDatabaseSetup() (queue.DatabaseSetup, error) {
	executable, err := os.Executable()
	if err != nil {
		return queue.DatabaseSetup{}, fmt.Errorf("locate release executable: %w", err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return queue.DatabaseSetup{}, fmt.Errorf("resolve release executable: %w", err)
	}
	return loadCoreDatabaseSetupBeside(executable)
}

func loadCoreDatabaseSetupBeside(executable string) (queue.DatabaseSetup, error) {
	path := filepath.Clean(filepath.Join(filepath.Dir(executable), "..", "database", "setup.sql"))
	setup, err := queue.LoadDatabaseSetup(path)
	if err != nil {
		return queue.DatabaseSetup{}, fmt.Errorf("load database setup: %w", err)
	}
	return setup, nil
}
