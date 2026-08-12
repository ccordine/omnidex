package queue

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestCognitionProposalMaterializationMigrationOwnsExactReplayAuthority(t *testing.T) {
	t.Parallel()
	var schema strings.Builder
	for _, name := range []string{
		"063_cognition_proposal_materialization.sql",
		"063_cognition_proposal_materialization_authority.sql",
		"063_cognition_proposal_materialization_trace.sql",
	} {
		raw, err := os.ReadFile("../../migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		schema.Write(raw)
	}
	for _, required := range []string{
		"CREATE TABLE cognition_proposal_materializations",
		"omnidex.cognition-proposal-materialization.v1",
		"reconciliation_id TEXT NOT NULL",
		"policy_call_id TEXT NOT NULL",
		"call_ordinal BIGINT NOT NULL",
		"proposal_index INTEGER NOT NULL",
		"proposal_kind TEXT NOT NULL",
		"source_kind TEXT NOT NULL",
		"pre_proposal_ledger_version BIGINT NOT NULL",
		"pre_proposal_ledger_sha256 TEXT NOT NULL",
		"pre_proposal_ledger_json TEXT NOT NULL",
		"mapping_id TEXT NOT NULL UNIQUE",
		"mapping_sha256 TEXT NOT NULL",
		"command_id TEXT NOT NULL CHECK",
		"UNIQUE (ledger_id,command_id)",
		"command_sha256 TEXT NOT NULL",
		"entry_uri TEXT NOT NULL",
		"output_ledger_version BIGINT NOT NULL",
		"output_ledger_status TEXT NOT NULL",
		"payload_json=cognition_canonical_jsonb(payload_json::jsonb)",
		"payload_json_sha256=encode(digest(payload_json,'sha256'),'hex')",
		"cognition_proposal_materializations_require_exact_authority",
		"calls.snapshot_sha256=NEW.snapshot_sha256",
		"calls.result_json::jsonb->>'decision_sha256'=NEW.decision_sha256",
		"cognition_reconciliations_require_proposal_materializations",
		"cognition_proposal_materializations_require_active_episode",
		"cognition_terminal_seals_require_proposal_materializations",
		"cognition_proposal_materializations_immutable",
	} {
		if !strings.Contains(schema.String(), required) {
			t.Fatalf("proposal materialization migration omitted %q", required)
		}
	}
}

func TestCognitionProposalMaterializationRejectsPayloadBeyondTraceCap(t *testing.T) {
	raw := bytes.Repeat([]byte{' '}, MaxCognitionTracePayloadBytes+1)
	if _, err := DecodeCognitionProposalMaterialization(
		raw, cognitionPayloadSHA(raw),
	); err == nil || !strings.Contains(err.Error(), "hard trace cap") {
		t.Fatalf("oversized proposal materialization error=%v", err)
	}
}

func TestCognitionProposalMaterializationHasOneTraceAndPersistencePath(t *testing.T) {
	t.Parallel()
	checks := map[string][]string{
		"cognition_proposal_materialization.go": {
			"PreProposalLedger           taskstate.MaterializedState",
			"func VerifyCognitionProposalMaterializationTrace(",
		},
		"cognition_runtime_reconciliation.go": {
			"newCognitionProposalMaterializations(",
			"insertCognitionProposalMaterializationsTx(",
			"requireCognitionProposalMaterializationReplayTx(",
		},
		"cognition_terminal_trace.go": {
			"'proposal_materialization'",
			"42 AS phase",
		},
		"cognition_sealed_trace_types.go": {
			"CognitionTraceKindProposalMaterialization",
		},
		"cognition_sealed_trace_payload.go": {
			"loadCognitionProposalMaterializationTracePayloadTx(",
		},
	}
	for name, required := range checks {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, token := range required {
			if !strings.Contains(string(raw), token) {
				t.Fatalf("%s omitted %q", name, token)
			}
		}
	}
}
