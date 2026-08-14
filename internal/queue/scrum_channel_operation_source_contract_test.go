package queue

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestScrumChannelOperationMigrationKeepsMinimalCausalReceipts(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/090_scrum_channel_operation_receipts.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	start := strings.Index(source, "CREATE TABLE scrum_channel_operations (")
	if start < 0 {
		t.Fatal("operation migration does not create its authoritative receipt table")
	}
	tableRemainder := source[start:]
	end := strings.Index(tableRemainder, "\n);")
	if end < 0 {
		t.Fatal("operation receipt table declaration is incomplete")
	}
	table := tableRemainder[:end]
	fieldPattern := regexp.MustCompile(`(?m)^\s{4}([a-z][a-z0-9_]*)\s+(?:TEXT|BIGINT|TIMESTAMPTZ)\b`)
	fieldMatches := fieldPattern.FindAllStringSubmatch(table, -1)
	fields := make([]string, 0, len(fieldMatches))
	for _, match := range fieldMatches {
		fields = append(fields, match[1])
	}
	wantFields := []string{
		"operation_id", "project_id", "card_id", "effect_kind",
		"effect_operation_id", "job_id", "result_action", "created_at",
	}
	if !slices.Equal(fields, wantFields) {
		t.Fatalf("operation receipt fields = %v, want minimal causal fields %v", fields, wantFields)
	}

	for _, required := range []string{
		"effect_operation_id TEXT NOT NULL UNIQUE",
		"NEW.effect_operation_id<>NEW.operation_id",
		"NEW.effect_operation_id<>scrum_effect_operation_id(",
		"FROM job_lifecycle_operations",
		"kind=NEW.effect_kind",
		"job.project_id=NEW.project_id AND job.pipeline='scrum'",
		"job.metadata->>'source'='omni-scrum'",
		"job.metadata->>'scrum_card_id'=NEW.card_id",
		"card.job_id=NEW.job_id::TEXT AND card.sync_job_id=NEW.job_id::TEXT",
		"card.column_name='in_progress' AND card.play_state='running'",
		"command_payload->>'feedback'=registry_payload->>'message'",
		"CREATE FUNCTION reject_operated_scrum_card_reuse()",
		"immutable operation receipt and cannot be reused",
		"octet_length(payload->>'message') BETWEEN 1 AND 4096",
		"CREATE FUNCTION scrum_channel_command_sha256(payload JSONB)",
		"registry_sha<>scrum_channel_command_sha256(registry_payload)",
		"public.digest(",
		"schema_name TEXT:=current_schema()",
		"ALTER FUNCTION %I.%s SET search_path TO pg_catalog, %I, public, pg_temp",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("operation migration is missing authority contract %q", required)
		}
	}
	for _, forbidden := range []string{
		"result_agent", "result_job", "result_card", "result_message",
		"archive", "backfill", "tombstone", "pg_trigger_depth",
	} {
		if strings.Contains(strings.ToLower(source), forbidden) {
			t.Errorf("operation migration contains forbidden preservation field/path %q", forbidden)
		}
	}

	oldActionLiteral := regexp.MustCompile(`["'\x60](steered|revised)["'\x60]`)
	for _, directory := range []string{".", "../api"} {
		err := filepath.WalkDir(directory, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			production, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			if token := oldActionLiteral.Find(production); token != nil {
				t.Errorf("production source %s retains old result action token %s", path, token)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
