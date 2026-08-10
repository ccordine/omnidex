package cognitiongauntlet

import (
	"fmt"
	"path/filepath"

	"github.com/gryph/omnidex/internal/queue"
)

// loadReleaseMigrationBundle derives the only permitted migration directory
// from the exact release executable. The queue package owns manifest parsing,
// bounded reads, immutable bytes, and installation authority.
func loadReleaseMigrationBundle(
	executable string,
	expectedManifestSHA256 string,
) (queue.MigrationBundle, error) {
	if executable == "" || filepath.Clean(executable) != executable ||
		!validDigest(expectedManifestSHA256) {
		return queue.MigrationBundle{}, fmt.Errorf("release migration bundle authority is invalid")
	}
	resolved, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return queue.MigrationBundle{}, fmt.Errorf("resolve release executable: %w", err)
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil || filepath.Base(filepath.Dir(resolved)) != "bin" {
		return queue.MigrationBundle{}, fmt.Errorf(
			"release executable is not inside the registered bin directory",
		)
	}
	directory := filepath.Join(filepath.Dir(filepath.Dir(resolved)), "migrations")
	return queue.LoadMigrationBundle(directory, expectedManifestSHA256)
}
