package queue

import (
	"strings"
	"testing"
)

func TestTelemetrySchemaDefinesCoreMetricsTablesAndIndexes(t *testing.T) {
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS omni_runs",
		"CREATE TABLE IF NOT EXISTS omni_run_events",
		"CREATE TABLE IF NOT EXISTS omni_model_calls",
		"CREATE TABLE IF NOT EXISTS omni_tool_calls",
		"CREATE TABLE IF NOT EXISTS omni_command_observations",
		"CREATE TABLE IF NOT EXISTS omni_objective_metrics",
		"CREATE TABLE IF NOT EXISTS omni_recovery_metrics",
		"CREATE TABLE IF NOT EXISTS omni_playbook_usage",
		"CREATE TABLE IF NOT EXISTS omni_benchmark_results",
		"CREATE TABLE IF NOT EXISTS omni_context_shrink_metrics",
		"CREATE TABLE IF NOT EXISTS omni_llm_context_usage",
		"UNIQUE(run_id, command_id)",
		"idx_omni_events_payload_gin",
		"idx_omni_model_role_model",
		"idx_omni_playbook_success",
		"idx_context_shrink_source_created",
		"idx_llm_context_usage_overloaded",
	} {
		if !strings.Contains(telemetrySchemaSQL, want) {
			t.Fatalf("telemetry schema missing %q", want)
		}
	}
}
