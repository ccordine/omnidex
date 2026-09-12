package db_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/database"
	"github.com/gryph/omnidex/internal/llm"
)

func TestAuthoritativeSetupDefinesLLMCallResultsWithoutHashReceipts(t *testing.T) {
	t.Parallel()
	setup := string(database.SetupSQL())
	for _, required := range []string{
		"CREATE TABLE llm_call_evidence",
		"model_input text NOT NULL",
		"output_continuation integer NOT NULL CHECK (output_continuation=0)",
		"dispatch_attempt integer NOT NULL CHECK (dispatch_attempt=1)",
		"replaces_call_evidence_id bigint CHECK (replaces_call_evidence_id IS NULL)",
		"octet_length(generation_receipt) BETWEEN 2 AND 16384",
		"raw_response_present boolean NOT NULL",
		fmt.Sprintf(
			"raw_response_bytes BETWEEN 0 AND %d",
			llm.MaxExactPreparedProviderResponseBytes+1,
		),
		fmt.Sprintf(
			"octet_length(candidate) <= %d",
			llm.MaxExactPreparedModelContentBytes,
		),
		"provider_duration_nanos",
		"context_tokens integer NOT NULL CHECK (context_tokens BETWEEN 1 AND 1048576)",
		"CREATE TABLE llm_call_outcomes",
		"CREATE TABLE llm_call_receipts",
		"'accepted','rejected','provider_failed','interrupted'",
		"validate_llm_call_outcome_insert",
		"llm_call_receipts_validate_insert",
		"llm_call_evidence_attempt_fkey",
	} {
		if !strings.Contains(setup, required) {
			t.Fatalf("authoritative setup omits %q", required)
		}
	}
	if strings.Contains(setup, "source_atomic_whole_leaf") {
		t.Fatal("authoritative setup retains forbidden whole-body correction authority")
	}
	for _, forbidden := range []string{
		"model_input_sha256", "provider_request_sha256", "generation_receipt_sha256",
		"raw_response_sha256", "validation_error_sha256", "prevent_llm_call_evidence_mutation",
		"source_base_sha256", "source_question_sha256",
		"system_envelope",
		"step attempt cannot complete after an unfinished or failed LLM call",
		"replacement dispatch lacks",
		"output continuation differs",
		"continuation.output_continuation=1",
	} {
		if strings.Contains(setup, forbidden) {
			t.Fatalf("authoritative setup retains forbidden repeated provider-call authority %q", forbidden)
		}
	}
}

func TestAuthoritativeSetupDefinesVerificationResultsWithoutHashOrPreservationGates(t *testing.T) {
	t.Parallel()
	setup := string(database.SetupSQL())
	for _, required := range []string{
		"CREATE TABLE verification_command_evidence",
		"'isolated_install','isolated_implementation','isolated_task','isolated_final'",
		"stdin_present boolean NOT NULL",
		"working_directory text NOT NULL",
		"duration_nanos",
		"stdout_complete boolean NOT NULL",
		"stderr_complete boolean NOT NULL",
		"verification_command_one_ordinal",
		"validate_verification_command_evidence_insert",
		"verification_command_evidence_attempt_fkey",
	} {
		if !strings.Contains(setup, required) {
			t.Fatalf("authoritative setup omits %q", required)
		}
	}
	for _, forbidden := range []string{
		"argv_sha256", "environment_sha256", "stdin_sha256", "stdout_sha256", "stderr_sha256",
		"workspace_sha256_before", "workspace_sha256_after", "workspace_changed",
		"prevent_verification_command_evidence_mutation",
		"step attempt cannot complete after failed verification command evidence",
	} {
		if strings.Contains(setup, forbidden) {
			t.Fatalf("setup retains obsolete verification gate %q", forbidden)
		}
	}
}
