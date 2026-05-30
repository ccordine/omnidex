package queue

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// embeddedSchemaMigrationCutoff is the last migrations/*.sql file whose changes are
// already applied via the embedded schemaSQL constants on startup. Existing databases
// that predate file-based migration tracking get 001..cutoff marked applied without
// re-running them.
const embeddedSchemaMigrationCutoff = "017_llm_context_ops.sql"

const schemaMigrationsTableSQL = `
CREATE TABLE IF NOT EXISTS schema_migrations (
    filename TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
`

// ResolveMigrationsDir returns the directory containing numbered SQL migrations.
func ResolveMigrationsDir() string {
	if v := strings.TrimSpace(os.Getenv("MIGRATIONS_DIR")); v != "" {
		if st, err := os.Stat(v); err == nil && st.IsDir() {
			return v
		}
	}
	candidates := []string{
		"migrations",
		"/app/migrations",
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(exeDir, "migrations"),
			filepath.Join(exeDir, "..", "migrations"),
		)
	}
	for _, candidate := range candidates {
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			return candidate
		}
	}
	return ""
}

func (r *Repository) ApplyFileMigrations(ctx context.Context, dir string) error {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil
	}
	if _, err := r.pool.Exec(ctx, schemaMigrationsTableSQL); err != nil {
		return fmt.Errorf("ensure schema_migrations table: %w", err)
	}

	files, err := listMigrationFiles(dir)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}

	if err := r.bootstrapFileMigrationLedger(ctx, files); err != nil {
		return err
	}

	applied, err := r.loadAppliedFileMigrations(ctx)
	if err != nil {
		return err
	}

	for _, name := range files {
		if applied[name] {
			continue
		}
		path := filepath.Join(dir, name)
		sqlText, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		body := strings.TrimSpace(string(sqlText))
		if body == "" {
			return fmt.Errorf("migration %s is empty", name)
		}
		if _, err := r.pool.Exec(ctx, body); err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err := r.pool.Exec(ctx, `INSERT INTO schema_migrations (filename) VALUES ($1)`, name); err != nil {
			return fmt.Errorf("record migration %s: %w", name, err)
		}
		log.Printf("schema migration applied: %s", name)
	}
	return nil
}

func listMigrationFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir %s: %w", dir, err)
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".sql") {
			continue
		}
		if !isNumberedMigrationFile(name) {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

func isNumberedMigrationFile(name string) bool {
	if len(name) < 5 {
		return false
	}
	for i := 0; i < 3 && i < len(name); i++ {
		if name[i] < '0' || name[i] > '9' {
			return false
		}
	}
	return name[3] == '_'
}

func (r *Repository) bootstrapFileMigrationLedger(ctx context.Context, files []string) error {
	var count int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	var jobsExists bool
	if err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.tables
			WHERE table_schema = current_schema()
			  AND table_name = 'jobs'
		)
	`).Scan(&jobsExists); err != nil {
		return err
	}
	if !jobsExists {
		return nil
	}

	for _, name := range files {
		if name > embeddedSchemaMigrationCutoff {
			continue
		}
		if _, err := r.pool.Exec(ctx, `
			INSERT INTO schema_migrations (filename)
			VALUES ($1)
			ON CONFLICT (filename) DO NOTHING
		`, name); err != nil {
			return fmt.Errorf("bootstrap migration ledger for %s: %w", name, err)
		}
	}
	return nil
}

func (r *Repository) loadAppliedFileMigrations(ctx context.Context) (map[string]bool, error) {
	rows, err := r.pool.Query(ctx, `SELECT filename FROM schema_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out[name] = true
	}
	return out, rows.Err()
}
