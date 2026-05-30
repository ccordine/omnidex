package queue

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListMigrationFilesIncludes018(t *testing.T) {
	dir := filepath.Join("..", "..", "migrations")
	files, err := listMigrationFiles(dir)
	if err != nil {
		t.Fatalf("listMigrationFiles: %v", err)
	}
	found := false
	for _, name := range files {
		if name == "018_scrum_card_llm_jobs.sql" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 018_scrum_card_llm_jobs.sql in %v", files)
	}
}

func TestResolveMigrationsDirFromRepo(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(wd, "..", ".."))
	t.Chdir(repoRoot)

	got := ResolveMigrationsDir()
	if got == "" {
		t.Fatal("expected migrations dir to resolve from repo root")
	}
	if filepath.Base(got) != "migrations" {
		t.Fatalf("ResolveMigrationsDir()=%q want basename migrations", got)
	}
}

func TestEmbeddedSchemaMigrationCutoffPrecedes018(t *testing.T) {
	if embeddedSchemaMigrationCutoff >= "018_scrum_card_llm_jobs.sql" {
		t.Fatalf("cutoff %q must sort before 018", embeddedSchemaMigrationCutoff)
	}
}
