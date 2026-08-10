package queue

import (
	"os"
	"strings"
	"testing"
)

func TestCognitionPolicyRecoveryMigrationOwnsExactCallAndAbandonmentAuthority(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/053_cognition_policy_recovery.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	for _, required := range []string{
		"migration 053 requires cognition_policy_calls to be empty",
		"CREATE OR REPLACE FUNCTION cognition_canonical_jsonb",
		"cognition_canonical_jsonb(attempt_json::jsonb-'id')",
		"attempt_json::jsonb->'runtime_budget'=runtime_budget_json::jsonb",
		"episodes.attested_brain_json::jsonb->'brain'=NEW.brain_json::jsonb",
		"CREATE TABLE cognition_policy_call_abandonments",
		"source_disposition IN ('expired','superseded')",
		"recovery_attempt BIGINT NOT NULL CHECK (recovery_attempt>source_attempt)",
		"calls.status='abandoned'",
		"recovery_attempt.status='active'",
		"abandoned cognition policy call has no typed disposition",
		"cognition_policy_call_abandonments_immutable",
	} {
		if !strings.Contains(schema, required) {
			t.Fatalf("policy recovery migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{"attempt_identity_json", "attempt_identity_sha256"} {
		if strings.Contains(schema, forbidden) {
			t.Fatalf("policy recovery migration retained formatting-dependent identity %q", forbidden)
		}
	}
}

func TestCognitionPolicyAbandonmentHasOneTypedStatusMutation(t *testing.T) {
	t.Parallel()
	files, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	owners := make([]string, 0, 1)
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".go") {
			continue
		}
		if file.Name() == "cognition_policy_recovery_schema_test.go" {
			continue
		}
		raw, err := os.ReadFile(file.Name())
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), "SET status='abandoned'") {
			owners = append(owners, file.Name())
		}
	}
	if len(owners) != 1 || owners[0] != "cognition_policy_abandonment.go" {
		t.Fatalf("untyped/duplicate abandonment status mutation owners=%v", owners)
	}
}
