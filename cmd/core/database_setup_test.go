package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoreDatabaseSetupUsesOnlyReleaseLayout(t *testing.T) {
	release := t.TempDir()
	directory := filepath.Join(release, "database")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	body := "-- Omnidex authoritative fresh-database setup.\n" +
		"-- __OMNIDEX_RUNTIME_SCHEMA__\nSELECT 1;\n"
	if err := os.WriteFile(filepath.Join(directory, "setup.sql"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadCoreDatabaseSetupBeside(filepath.Join(release, "bin", "agent-core")); err != nil {
		t.Fatal(err)
	}
}

func TestCoreDatabaseSetupHasNoEnvironmentSelectedPath(t *testing.T) {
	raw, err := os.ReadFile("database_setup.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "LookupEnv") {
		t.Fatal("core database setup path remains environment-selectable")
	}
}
