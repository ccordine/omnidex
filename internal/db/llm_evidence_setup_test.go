package db_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/database"
	"github.com/gryph/omnidex/internal/llm"
)

func TestAuthoritativeSetupDefinesExactImmutableLLMCallEvidence(t *testing.T) {
	t.Parallel()
	setup := string(database.SetupSQL())
	for _, required := range []string{
		"CREATE TABLE llm_call_evidence",
		"system_envelope text NOT NULL",
		"model_input_sha256",
		"dispatch_attempt integer NOT NULL CHECK (dispatch_attempt BETWEEN 1 AND 2)",
		"replaces_call_evidence_id bigint",
		"provider_request_sha256",
		"generation_receipt_sha256",
		"octet_length(generation_receipt) BETWEEN 2 AND 16384",
		"raw_response_present boolean NOT NULL",
		"raw_response_sha256",
		fmt.Sprintf(
			"raw_response_bytes BETWEEN 0 AND %d",
			llm.MaxExactPreparedProviderResponseBytes+1,
		),
		fmt.Sprintf(
			"octet_length(candidate) <= %d",
			llm.MaxExactPreparedModelContentBytes,
		),
		"error_sha256",
		"provider_duration_nanos",
		"context_tokens integer NOT NULL CHECK (context_tokens BETWEEN 1 AND 1048576)",
		"CREATE TABLE llm_call_outcomes",
		"CREATE TABLE llm_call_receipts",
		"'accepted','rejected','provider_failed','interrupted'",
		"validation_error_sha256",
		"validate_llm_call_outcome_insert",
		"llm_call_evidence_immutable",
		"llm_call_evidence_truncate_immutable",
		"llm_call_outcomes_immutable",
		"llm_call_outcomes_truncate_immutable",
		"llm_call_receipts_validate_insert",
		"llm_call_receipts_immutable",
		"llm_call_receipts_truncate_immutable",
		"llm_call_evidence_attempt_fkey",
		"step attempt cannot complete after failed verification command evidence",
	} {
		if !strings.Contains(setup, required) {
			t.Fatalf("authoritative setup omits %q", required)
		}
	}
	if strings.Contains(setup, "source_atomic_whole_leaf") {
		t.Fatal("authoritative setup retains forbidden whole-body correction authority")
	}
}

func TestAuthoritativeSetupDefinesExactImmutableVerificationCommands(t *testing.T) {
	t.Parallel()
	setup := string(database.SetupSQL())
	for _, required := range []string{
		"CREATE TABLE verification_command_evidence",
		"'isolated_install','isolated_implementation','isolated_task','isolated_final'",
		"argv_sha256",
		"environment_sha256",
		"stdin_present boolean NOT NULL",
		"working_directory text NOT NULL",
		"duration_nanos",
		"stdout_complete boolean NOT NULL",
		"stdout_sha256",
		"stderr_complete boolean NOT NULL",
		"stderr_sha256",
		"workspace_sha256_before",
		"verification_command_one_ordinal",
		"validate_verification_command_evidence_insert",
		"verification_command_evidence_immutable",
		"verification_command_evidence_truncate_immutable",
		"verification_command_evidence_attempt_fkey",
	} {
		if !strings.Contains(setup, required) {
			t.Fatalf("authoritative setup omits %q", required)
		}
	}
}
