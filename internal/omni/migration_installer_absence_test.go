package omni

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHostCLIHasNoAlternateMigrationInstaller(t *testing.T) {
	for _, name := range []string{"migrate.go", "migrate_command.go", "pgsql_memory.go"} {
		if _, err := os.Stat(name); !os.IsNotExist(err) {
			t.Fatalf("obsolete migration implementation %s still exists", name)
		}
	}
	for _, name := range []string{
		filepath.Join("..", "..", "database", "migrations", ".gitkeep"),
		filepath.Join("..", "..", "docs", "MERGE_NOTES.md"),
		filepath.Join("..", "..", "docs", "omni", "CONTRACTS.md"),
		filepath.Join("..", "..", "docs", "omni", "DEV_BIBLE.md"),
		filepath.Join("..", "..", "docs", "omni", "ROADMAP.md"),
	} {
		if _, err := os.Stat(name); !os.IsNotExist(err) {
			t.Fatalf("obsolete duplicate architecture %s still exists", name)
		}
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		raw, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"runMigrate(", "RunMigrate", "DiscoverMigrations", "DefaultMigrationDBConfig",
			"PsqlExecutor", "omni_migrations", "manage database migrations",
			"EnsureSchema(", "CREATE TABLE", "ALTER TABLE", "CREATE INDEX",
			"PGMemoryStore", "NewPGMemoryStore", "gin_trgm_ops",
		} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatalf("%s retains alternate migration path %q", entry.Name(), forbidden)
			}
		}
	}
}
