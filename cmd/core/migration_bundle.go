package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/version"
)

func loadCoreMigrationBundle() (queue.MigrationBundle, error) {
	executable, err := os.Executable()
	if err != nil {
		return queue.MigrationBundle{}, fmt.Errorf("locate release executable: %w", err)
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return queue.MigrationBundle{}, fmt.Errorf("resolve release executable: %w", err)
	}
	return loadCoreMigrationBundleBeside(executable)
}

func loadCoreMigrationBundleBeside(executable string) (queue.MigrationBundle, error) {
	directory := filepath.Clean(filepath.Join(filepath.Dir(executable), "..", "migrations"))
	bundle, err := queue.LoadMigrationBundle(directory, version.MigrationsSHA256)
	if err != nil {
		return queue.MigrationBundle{}, fmt.Errorf("load migration authority: %w", err)
	}
	return bundle, nil
}
