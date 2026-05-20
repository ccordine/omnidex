package omni

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var migrationFileRe = regexp.MustCompile(`^([0-9]{14})_([a-z0-9_]{3,120})\.(up|down)\.sql$`)

type MigrationFile struct {
	Version  string
	Name     string
	UpPath   string
	DownPath string
	Checksum string
}

type AppliedMigration struct {
	Version  string
	Batch    int
	Checksum string
}

type MigrationDBConfig struct {
	Mode      string
	Container string
	Host      string
	Port      string
	Database  string
	User      string
	Password  string
	SSLMode   string
}

func DefaultMigrationDBConfig() MigrationDBConfig {
	cfg := MigrationDBConfig{
		Mode:      envOrDefault("OMNI_DB_MODE", "docker_exec"),
		Container: envOrDefault("OMNI_PG_CONTAINER", "postgres_db"),
		Host:      envOrDefault("OMNI_DB_HOST", "127.0.0.1"),
		Port:      envOrDefault("OMNI_DB_PORT", "5432"),
		Database:  envOrDefault("OMNI_DB_NAME", "postgres"),
		User:      envOrDefault("OMNI_DB_USER", "omnidex"),
		Password:  os.Getenv("OMNI_DB_PASSWORD"),
		SSLMode:   envOrDefault("OMNI_DB_SSLMODE", "disable"),
	}
	if cfg.Password == "" {
		cfg.Password = os.Getenv("POSTGRES_PASSWORD")
	}
	if cfg.Mode != "docker_exec" && cfg.Mode != "direct" {
		cfg.Mode = "docker_exec"
	}
	return cfg
}

func RunMigrateCreate(migrationsDir, rawName string) (string, string, error) {
	name := normalizeMigrationName(rawName)
	if name == "" {
		return "", "", fmt.Errorf("migration name is required")
	}
	if err := os.MkdirAll(migrationsDir, 0o755); err != nil {
		return "", "", err
	}

	version := time.Now().UTC().Format("20060102150405")
	upPath := filepath.Join(migrationsDir, fmt.Sprintf("%s_%s.up.sql", version, name))
	downPath := filepath.Join(migrationsDir, fmt.Sprintf("%s_%s.down.sql", version, name))

	upStub := "-- Up migration: " + name + "\n" +
		"-- Add forward SQL statements below.\n\n"
	downStub := "-- Down migration: " + name + "\n" +
		"-- Add rollback SQL statements below.\n\n"

	if err := os.WriteFile(upPath, []byte(upStub), 0o644); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(downPath, []byte(downStub), 0o644); err != nil {
		return "", "", err
	}

	return upPath, downPath, nil
}

func RunMigrateStatus(migrationsDir string, cfg MigrationDBConfig) (string, error) {
	migrations, err := DiscoverMigrations(migrationsDir)
	if err != nil {
		return "", err
	}

	exec := NewPsqlExecutor(cfg)
	if err := exec.EnsureMigrationTable(); err != nil {
		return "", err
	}

	applied, err := exec.AppliedMigrations()
	if err != nil {
		return "", err
	}

	appliedSet := map[string]AppliedMigration{}
	for _, a := range applied {
		appliedSet[a.Version] = a
	}

	lines := make([]string, 0, len(migrations)+1)
	lines = append(lines, "Version | Name | Status | Batch")
	for _, migration := range migrations {
		status := "pending"
		batch := "-"
		if row, ok := appliedSet[migration.Version]; ok {
			status = "applied"
			batch = strconv.Itoa(row.Batch)
		}
		lines = append(lines, fmt.Sprintf("%s | %s | %s | %s", migration.Version, migration.Name, status, batch))
	}
	if len(migrations) == 0 {
		lines = append(lines, "(no migration files found)")
	}

	return strings.Join(lines, "\n"), nil
}

func RunMigrateUp(migrationsDir string, cfg MigrationDBConfig, steps int) (string, error) {
	if steps <= 0 {
		steps = 1000000
	}

	migrations, err := DiscoverMigrations(migrationsDir)
	if err != nil {
		return "", err
	}
	exec := NewPsqlExecutor(cfg)
	if err := exec.EnsureMigrationTable(); err != nil {
		return "", err
	}
	applied, err := exec.AppliedMigrations()
	if err != nil {
		return "", err
	}

	appliedSet := map[string]AppliedMigration{}
	for _, a := range applied {
		appliedSet[a.Version] = a
	}

	batch, err := exec.NextBatch()
	if err != nil {
		return "", err
	}

	appliedCount := 0
	for _, migration := range migrations {
		if _, ok := appliedSet[migration.Version]; ok {
			continue
		}
		if appliedCount >= steps {
			break
		}

		sqlText, err := os.ReadFile(migration.UpPath)
		if err != nil {
			return "", err
		}
		statement := strings.TrimSpace(string(sqlText))
		if statement == "" {
			return "", fmt.Errorf("up migration is empty: %s", migration.UpPath)
		}

		runSQL := strings.Join([]string{
			"BEGIN;",
			statement,
			fmt.Sprintf("INSERT INTO omni_migrations (version, name, batch, checksum_sha256, applied_at) VALUES ('%s', '%s', %d, '%s', NOW());", migration.Version, migration.Name, batch, migration.Checksum),
			"COMMIT;",
		}, "\n")

		if err := exec.ExecSQL(runSQL); err != nil {
			return "", fmt.Errorf("apply migration %s failed: %w", migration.Version, err)
		}
		appliedCount++
	}

	if appliedCount == 0 {
		return "No pending migrations.", nil
	}
	return fmt.Sprintf("Applied %d migration(s) in batch %d.", appliedCount, batch), nil
}

func RunMigrateDown(migrationsDir string, cfg MigrationDBConfig, steps int) (string, error) {
	if steps <= 0 {
		steps = 1
	}

	migrations, err := DiscoverMigrations(migrationsDir)
	if err != nil {
		return "", err
	}
	exec := NewPsqlExecutor(cfg)
	if err := exec.EnsureMigrationTable(); err != nil {
		return "", err
	}
	applied, err := exec.AppliedMigrations()
	if err != nil {
		return "", err
	}
	if len(applied) == 0 {
		return "No applied migrations to roll back.", nil
	}

	versions := make([]AppliedMigration, len(applied))
	copy(versions, applied)
	sort.Slice(versions, func(i, j int) bool {
		if versions[i].Batch == versions[j].Batch {
			return versions[i].Version > versions[j].Version
		}
		return versions[i].Batch > versions[j].Batch
	})

	fileByVersion := map[string]MigrationFile{}
	for _, migration := range migrations {
		fileByVersion[migration.Version] = migration
	}

	rolledBack := 0
	for _, row := range versions {
		if rolledBack >= steps {
			break
		}
		migration, ok := fileByVersion[row.Version]
		if !ok {
			return "", fmt.Errorf("missing migration files for applied version %s", row.Version)
		}

		downSQL, err := os.ReadFile(migration.DownPath)
		if err != nil {
			return "", err
		}
		statement := strings.TrimSpace(string(downSQL))
		if statement == "" {
			return "", fmt.Errorf("down migration is empty: %s", migration.DownPath)
		}

		runSQL := strings.Join([]string{
			"BEGIN;",
			statement,
			fmt.Sprintf("DELETE FROM omni_migrations WHERE version = '%s';", migration.Version),
			"COMMIT;",
		}, "\n")
		if err := exec.ExecSQL(runSQL); err != nil {
			return "", fmt.Errorf("rollback migration %s failed: %w", migration.Version, err)
		}
		rolledBack++
	}

	if rolledBack == 0 {
		return "No migrations rolled back.", nil
	}
	return fmt.Sprintf("Rolled back %d migration(s).", rolledBack), nil
}

func DiscoverMigrations(migrationsDir string) ([]MigrationFile, error) {
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []MigrationFile{}, nil
		}
		return nil, err
	}

	type pair struct {
		version  string
		name     string
		upPath   string
		downPath string
	}

	pairs := map[string]pair{}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := migrationFileRe.FindStringSubmatch(entry.Name())
		if len(match) != 4 {
			continue
		}

		version := match[1]
		name := match[2]
		direction := match[3]
		key := version + "_" + name
		current := pairs[key]
		current.version = version
		current.name = name

		fullPath := filepath.Join(migrationsDir, entry.Name())
		if direction == "up" {
			current.upPath = fullPath
		} else {
			current.downPath = fullPath
		}
		pairs[key] = current
	}

	migrations := make([]MigrationFile, 0, len(pairs))
	for _, p := range pairs {
		if p.upPath == "" || p.downPath == "" {
			return nil, fmt.Errorf("migration %s_%s is missing up/down pair", p.version, p.name)
		}
		checksum, err := checksumFiles(p.upPath, p.downPath)
		if err != nil {
			return nil, err
		}
		migrations = append(migrations, MigrationFile{
			Version:  p.version,
			Name:     p.name,
			UpPath:   p.upPath,
			DownPath: p.downPath,
			Checksum: checksum,
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		if migrations[i].Version == migrations[j].Version {
			return migrations[i].Name < migrations[j].Name
		}
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

func checksumFiles(upPath, downPath string) (string, error) {
	up, err := os.ReadFile(upPath)
	if err != nil {
		return "", err
	}
	down, err := os.ReadFile(downPath)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(string(up) + "\n---\n" + string(down)))
	return hex.EncodeToString(sum[:]), nil
}

type PsqlExecutor struct {
	cfg MigrationDBConfig
}

func NewPsqlExecutor(cfg MigrationDBConfig) PsqlExecutor {
	return PsqlExecutor{cfg: cfg}
}

func (p PsqlExecutor) EnsureMigrationTable() error {
	sql := `
CREATE TABLE IF NOT EXISTS omni_migrations (
  version VARCHAR(14) PRIMARY KEY,
  name TEXT NOT NULL,
  batch INTEGER NOT NULL,
  checksum_sha256 CHAR(64) NOT NULL,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
`
	return p.ExecSQL(sql)
}

func (p PsqlExecutor) AppliedMigrations() ([]AppliedMigration, error) {
	raw, err := p.QuerySQL("SELECT version, batch, checksum_sha256 FROM omni_migrations ORDER BY version;")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(raw) == "" {
		return []AppliedMigration{}, nil
	}

	lines := strings.Split(strings.TrimSpace(raw), "\n")
	out := make([]AppliedMigration, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) != 3 {
			return nil, fmt.Errorf("unexpected migration row format: %s", line)
		}
		batch, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, fmt.Errorf("invalid batch value %q: %w", parts[1], err)
		}
		out = append(out, AppliedMigration{
			Version:  strings.TrimSpace(parts[0]),
			Batch:    batch,
			Checksum: strings.TrimSpace(parts[2]),
		})
	}
	return out, nil
}

func (p PsqlExecutor) NextBatch() (int, error) {
	raw, err := p.QuerySQL("SELECT COALESCE(MAX(batch), 0) FROM omni_migrations;")
	if err != nil {
		return 0, err
	}
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("invalid max batch value %q: %w", raw, err)
	}
	return v + 1, nil
}

func (p PsqlExecutor) ExecSQL(sql string) error {
	_, err := p.run(sql, false)
	return err
}

func (p PsqlExecutor) QuerySQL(sql string) (string, error) {
	return p.run(sql, true)
}

func (p PsqlExecutor) run(sql string, tuplesOnly bool) (string, error) {
	args := []string{}
	if p.cfg.Mode == "docker_exec" {
		args = append(args, "docker", "exec", "-i", p.cfg.Container, "psql")
	} else {
		args = append(args, "psql")
	}

	args = append(args, "-X", "-v", "ON_ERROR_STOP=1")
	if tuplesOnly {
		args = append(args, "-At", "-F", "|")
	}

	if p.cfg.Mode == "direct" {
		args = append(args, "-h", p.cfg.Host, "-p", p.cfg.Port)
	}
	args = append(args, "-U", p.cfg.User, "-d", p.cfg.Database, "-c", sql)

	cmd := exec.Command(args[0], args[1:]...)
	if p.cfg.Mode == "direct" {
		env := os.Environ()
		if strings.TrimSpace(p.cfg.Password) != "" {
			env = append(env, "PGPASSWORD="+p.cfg.Password)
		}
		if strings.TrimSpace(p.cfg.SSLMode) != "" {
			env = append(env, "PGSSLMODE="+p.cfg.SSLMode)
		}
		cmd.Env = env
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("psql command failed: %w; stderr=%s", err, strings.TrimSpace(stderr.String()))
	}

	return strings.TrimSpace(stdout.String()), nil
}

func normalizeMigrationName(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	raw = strings.ReplaceAll(raw, "-", "_")
	raw = strings.ReplaceAll(raw, " ", "_")
	filtered := strings.Builder{}
	for _, r := range raw {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			filtered.WriteRune(r)
		}
	}
	name := strings.Trim(filtered.String(), "_")
	for strings.Contains(name, "__") {
		name = strings.ReplaceAll(name, "__", "_")
	}
	if len(name) > 120 {
		name = name[:120]
	}
	return name
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
