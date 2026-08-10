package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/version"
)

func TestCoreMigrationBundleUsesOnlyReleaseLayout(t *testing.T) {
	release := t.TempDir()
	directory := filepath.Join(release, "migrations")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	body := []byte("SELECT 1;\n")
	bodyDigest := sha256.Sum256(body)
	manifest := []byte(fmt.Sprintf(
		"%s  001_probe.sql\n", hex.EncodeToString(bodyDigest[:]),
	))
	manifestDigest := sha256.Sum256(manifest)
	if err := os.WriteFile(filepath.Join(directory, "001_probe.sql"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "SHA256SUMS"), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	priorDigest := version.MigrationsSHA256
	version.MigrationsSHA256 = hex.EncodeToString(manifestDigest[:])
	t.Cleanup(func() { version.MigrationsSHA256 = priorDigest })
	if _, err := loadCoreMigrationBundleBeside(filepath.Join(release, "bin", "agent-core")); err != nil {
		t.Fatal(err)
	}
}

func TestCoreMigrationBundleHasNoEnvironmentSelectedPath(t *testing.T) {
	raw, err := os.ReadFile("migration_bundle.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "MIGRATIONS"+"_DIR") ||
		strings.Contains(string(raw), "LookupEnv") {
		t.Fatal("core migration authority remains environment-selectable")
	}
}
