package queue

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/db"
)

const (
	databaseSetupName              = "setup.sql"
	databaseSetupRuntimeSchemaSlot = "__OMNIDEX_RUNTIME_SCHEMA__"
	maxDatabaseSetupBytes          = 16 * 1024 * 1024
)

// DatabaseSetup is the immutable current database definition loaded from the
// sole authoritative setup SQL file.
type DatabaseSetup struct {
	body []byte
}

// LoadDatabaseSetup reads exactly one bounded, real setup.sql file. There is
// no directory scan, version history, manifest, digest, or upgrade authority.
func LoadDatabaseSetup(exactPath string) (DatabaseSetup, error) {
	if exactPath == "" || !filepath.IsAbs(exactPath) ||
		filepath.Clean(exactPath) != exactPath || filepath.Base(exactPath) != databaseSetupName {
		return DatabaseSetup{}, fmt.Errorf("database setup requires one exact absolute setup.sql path")
	}
	initial, err := os.Lstat(exactPath)
	if err != nil {
		return DatabaseSetup{}, fmt.Errorf("inspect database setup: %w", err)
	}
	if initial.Mode()&os.ModeSymlink != 0 || !initial.Mode().IsRegular() ||
		initial.Size() <= 0 || initial.Size() > maxDatabaseSetupBytes {
		return DatabaseSetup{}, fmt.Errorf("database setup is not one bounded regular file")
	}
	file, err := os.Open(exactPath)
	if err != nil {
		return DatabaseSetup{}, fmt.Errorf("open database setup: %w", err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(initial, opened) {
		return DatabaseSetup{}, fmt.Errorf("database setup changed while opening")
	}
	body, err := io.ReadAll(io.LimitReader(file, maxDatabaseSetupBytes+1))
	if err != nil {
		return DatabaseSetup{}, fmt.Errorf("read database setup: %w", err)
	}
	if len(body) == 0 || len(body) > maxDatabaseSetupBytes {
		return DatabaseSetup{}, fmt.Errorf("database setup exceeds its exact size boundary")
	}
	final, err := os.Lstat(exactPath)
	if err != nil || final.Mode()&os.ModeSymlink != 0 || !os.SameFile(initial, final) ||
		final.Size() != int64(len(body)) {
		return DatabaseSetup{}, fmt.Errorf("database setup changed while reading")
	}
	setup := DatabaseSetup{body: append([]byte{}, body...)}
	if err := setup.validate(); err != nil {
		return DatabaseSetup{}, err
	}
	return setup, nil
}

func (s DatabaseSetup) validate() error {
	if len(s.body) == 0 || len(s.body) > maxDatabaseSetupBytes {
		return fmt.Errorf("database setup bytes are unavailable")
	}
	raw := string(s.body)
	if !strings.HasPrefix(raw, "-- Omnidex authoritative fresh-database setup.\n") {
		return fmt.Errorf("database setup lacks its current-state authority header")
	}
	if !strings.Contains(raw, databaseSetupRuntimeSchemaSlot) {
		return fmt.Errorf("database setup lacks its runtime schema slot")
	}
	return nil
}

func (s DatabaseSetup) render(runtimeSchema string) ([]byte, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	if err := db.ValidateRuntimeSchemaName(runtimeSchema); err != nil {
		return nil, err
	}
	rendered := bytes.ReplaceAll(
		s.body,
		[]byte(databaseSetupRuntimeSchemaSlot),
		[]byte(runtimeSchema),
	)
	if bytes.Contains(rendered, []byte(databaseSetupRuntimeSchemaSlot)) {
		return nil, fmt.Errorf("database setup runtime schema slot was not fully resolved")
	}
	return rendered, nil
}
