package omni

import (
	"flag"
	"fmt"
	"path/filepath"
	"strings"
)

func (a *App) runMigrate(args []string) error {
	fs := flag.NewFlagSet("omni migrate", flag.ContinueOnError)
	fs.SetOutput(a.errOut)
	dir := fs.String("dir", filepath.Join("database", "migrations"), "migration directory")
	steps := fs.Int("steps", 0, "number of migration steps")
	dbMode := fs.String("db-mode", "", "database mode: docker_exec|direct")
	dbContainer := fs.String("db-container", "", "docker container name")
	dbHost := fs.String("db-host", "", "database host")
	dbPort := fs.String("db-port", "", "database port")
	dbName := fs.String("db-name", "", "database name")
	dbUser := fs.String("db-user", "", "database user")
	dbPassword := fs.String("db-password", "", "database password")
	dbSSLMode := fs.String("db-sslmode", "", "database SSL mode")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("migration subcommand is required: create, up, down, or status")
	}

	cfg := DefaultMigrationDBConfig()
	overrideString(&cfg.Mode, *dbMode)
	overrideString(&cfg.Container, *dbContainer)
	overrideString(&cfg.Host, *dbHost)
	overrideString(&cfg.Port, *dbPort)
	overrideString(&cfg.Database, *dbName)
	overrideString(&cfg.User, *dbUser)
	overrideString(&cfg.Password, *dbPassword)
	overrideString(&cfg.SSLMode, *dbSSLMode)
	migrationsDir := strings.TrimSpace(*dir)
	if migrationsDir == "" {
		return fmt.Errorf("migration directory is required")
	}

	switch fs.Arg(0) {
	case "create":
		if fs.NArg() != 2 {
			return fmt.Errorf("migration create requires exactly one name")
		}
		upPath, downPath, err := RunMigrateCreate(migrationsDir, fs.Arg(1))
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(a.out, "Created migration files:\n- %s\n- %s\n", upPath, downPath)
		return err
	case "status":
		if fs.NArg() != 1 {
			return fmt.Errorf("migration status accepts no positional arguments")
		}
		result, err := RunMigrateStatus(migrationsDir, cfg)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(a.out, result)
		return err
	case "up":
		if fs.NArg() != 1 {
			return fmt.Errorf("migration up accepts no positional arguments")
		}
		result, err := RunMigrateUp(migrationsDir, cfg, *steps)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(a.out, result)
		return err
	case "down":
		if fs.NArg() != 1 {
			return fmt.Errorf("migration down accepts no positional arguments")
		}
		result, err := RunMigrateDown(migrationsDir, cfg, *steps)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(a.out, result)
		return err
	default:
		return fmt.Errorf("unknown migration subcommand %q", fs.Arg(0))
	}
}

func overrideString(target *string, value string) {
	if value = strings.TrimSpace(value); value != "" {
		*target = value
	}
}
