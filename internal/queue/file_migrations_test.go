package queue

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrationInstallerHasOneBundleAuthorityAndNoEmbeddedBootstrap(t *testing.T) {
	files := []string{
		"file_migrations.go", "migration_bundle.go", "repository.go",
		"repository_migrate_fresh.go",
	}
	for _, name := range files {
		raw, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"ResolveMigrationsDir", "ApplyFileMigrations", "schemaSQL",
			"v3SchemaSQL", "telemetrySchemaSQL", "projectsUISchemaSQL",
			"embeddedSchemaMigrationCutoff",
		} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatalf("%s retains forbidden migration fallback %q", name, forbidden)
			}
		}
	}
	for _, removed := range []string{"schema.go", "v3_schema.go", "telemetry_schema.go"} {
		if _, err := os.Stat(filepath.Join(".", removed)); !os.IsNotExist(err) {
			t.Fatalf("embedded bootstrap file %s still exists", removed)
		}
	}
}

func TestAppliedMigrationValidationRejectsUnknownGapAndAuthorityDrift(t *testing.T) {
	body1 := []byte("SELECT 1;\n")
	body2 := []byte("SELECT 2;\n")
	bundle := migrationBundleForValidationTest(body1, body2)
	exact := map[string]appliedMigration{
		"001_first.sql": {sha256: digestMigrationBytes(body1), manifestSHA256: bundle.manifestSHA256},
		"002_next.sql":  {sha256: digestMigrationBytes(body2), manifestSHA256: bundle.manifestSHA256},
	}
	if err := validateAppliedFileMigrations(bundle, exact, true); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(map[string]appliedMigration){
		"unknown": func(rows map[string]appliedMigration) {
			rows["003_unknown.sql"] = appliedMigration{sha256: strings.Repeat("a", 64), manifestSHA256: bundle.manifestSHA256}
		},
		"missing lower": func(rows map[string]appliedMigration) { delete(rows, "001_first.sql") },
		"body digest drift": func(rows map[string]appliedMigration) {
			row := rows["001_first.sql"]
			row.sha256 = strings.Repeat("a", 64)
			rows["001_first.sql"] = row
		},
		"missing body digest": func(rows map[string]appliedMigration) {
			row := rows["001_first.sql"]
			row.sha256 = ""
			rows["001_first.sql"] = row
		},
		"missing manifest": func(rows map[string]appliedMigration) {
			row := rows["001_first.sql"]
			row.manifestSHA256 = ""
			rows["001_first.sql"] = row
		},
		"manifest drift": func(rows map[string]appliedMigration) {
			row := rows["001_first.sql"]
			row.manifestSHA256 = strings.Repeat("a", 64)
			rows["001_first.sql"] = row
		},
	} {
		t.Run(name, func(t *testing.T) {
			rows := make(map[string]appliedMigration, len(exact))
			for key, value := range exact {
				rows[key] = value
			}
			mutate(rows)
			if err := validateAppliedFileMigrations(bundle, rows, true); err == nil {
				t.Fatal("invalid applied migration ledger was accepted")
			}
		})
	}
}

func migrationBundleForValidationTest(bodies ...[]byte) MigrationBundle {
	entries := make([]migrationBundleEntry, 0, len(bodies))
	var manifest strings.Builder
	for index, body := range bodies {
		name := []string{"001_first.sql", "002_next.sql"}[index]
		digest := digestMigrationBytes(body)
		entries = append(entries, migrationBundleEntry{name: name, sha256: digest, body: body})
		manifest.WriteString(digest + "  " + name + "\n")
	}
	raw := []byte(manifest.String())
	return MigrationBundle{
		manifestSHA256: digestMigrationBytes(raw), manifest: raw, entries: entries,
	}
}
