package queue

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/db"
)

const (
	databaseSetupRuntimeSchemaSlot = "__OMNIDEX_RUNTIME_SCHEMA__"
	maxDatabaseSetupBytes          = 16 * 1024 * 1024
)

func validateDatabaseSetup(body []byte) error {
	if len(body) == 0 || len(body) > maxDatabaseSetupBytes {
		return fmt.Errorf("database setup bytes are unavailable")
	}
	raw := string(body)
	if !strings.HasPrefix(raw, "-- Omnidex authoritative fresh-database setup.\n") {
		return fmt.Errorf("database setup lacks its current-state authority header")
	}
	if !strings.Contains(raw, databaseSetupRuntimeSchemaSlot) {
		return fmt.Errorf("database setup lacks its runtime schema slot")
	}
	return nil
}

func renderDatabaseSetup(body []byte, runtimeSchema string) ([]byte, error) {
	if err := validateDatabaseSetup(body); err != nil {
		return nil, err
	}
	if err := db.ValidateRuntimeSchemaName(runtimeSchema); err != nil {
		return nil, err
	}
	rendered := bytes.ReplaceAll(
		body,
		[]byte(databaseSetupRuntimeSchemaSlot),
		[]byte(runtimeSchema),
	)
	if bytes.Contains(rendered, []byte(databaseSetupRuntimeSchemaSlot)) {
		return nil, fmt.Errorf("database setup runtime schema slot was not fully resolved")
	}
	return rendered, nil
}
