package queue

import (
	"os"
	"strings"
	"testing"
)

func TestStationTerminalReceiptMigrationIsHashGuardedAndFailClosed(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile("../../migrations/075_station_terminal_receipt_authority.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	for _, required := range []string{
		"446e4967f4e6d51aedf280338a5e956e074d9d8c4eb005c4cf612f7cb3d2b8cd",
		"56cf275de4cdef8114fa9969f419cf35fb022cff2bdb7bfc7245890090c187ed",
		"487257a8fa2248b32747d6e7342d8bd370b0c914f291d8229472ea3bf4911311",
		"provider_request_disposition'<>'dispatched'",
		"authority_'||attempt_status",
		"evidence_count<>call_count",
		"validate_station_llm_call_evidence_identity",
		"cannot classify historical failed provider discovery receipts",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("migration lacks %q", required)
		}
	}
	for _, forbidden := range []string{"CASCADE", "DROP TRIGGER IF EXISTS", "fallback", "legacy"} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("migration contains forbidden %q", forbidden)
		}
	}
}
