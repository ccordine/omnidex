package queue

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/taskstate"
)

func TestPostgresCognitionProposalMaterializationPortableVerifierRederivesExactMutation(t *testing.T) {
	fixture, raw := cognitionProposalMaterializationFixture(t)
	value, err := DecodeCognitionProposalMaterialization(raw, cognitionPayloadSHA(raw))
	if err != nil {
		t.Fatal(err)
	}
	var commandRaw []byte
	var reconciliationID, policyCallID string
	var callOrdinal uint64
	if err := fixture.Repository.pool.QueryRow(t.Context(), `
		SELECT reconciliations.command_json,reconciliations.reconciliation_id,
		       reconciliations.policy_call_id,snapshots.call_ordinal
		FROM cognition_reconciliations reconciliations
		JOIN cognition_runtime_snapshots snapshots
		  ON snapshots.snapshot_sha256=reconciliations.snapshot_sha256
		WHERE reconciliations.reconciliation_id=$1
	`, value.ReconciliationID).Scan(
		&commandRaw, &reconciliationID, &policyCallID, &callOrdinal,
	); err != nil {
		t.Fatal(err)
	}
	var command cognitionruntime.ReconciliationCommand
	if err := json.Unmarshal(commandRaw, &command); err != nil {
		t.Fatal(err)
	}
	tx, err := fixture.Repository.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	episode, found, err := loadCognitionEpisodeTx(t.Context(), tx, fixture.EpisodeID, false)
	if err != nil || !found {
		t.Fatalf("load proposal materialization episode: found=%t error=%v", found, err)
	}
	snapshot, _, err := loadCognitionProposalMaterializationSnapshotTx(
		t.Context(), tx, episode, value.SnapshotSHA256,
	)
	if err != nil {
		t.Fatal(err)
	}
	binding := CognitionProposalMaterializationTraceAuthority{
		ReconciliationID: reconciliationID, PolicyCallID: policyCallID,
		CallOrdinal: callOrdinal, Phase: 42, Sequence: int64(value.ProposalIndex),
		ID: value.ID, SHA256: cognitionPayloadSHA(raw),
	}
	if err := VerifyCognitionProposalMaterializationTrace(
		value, binding, snapshot, command.Decision, command.ActionSchema,
	); err != nil {
		t.Fatalf("portable proposal materialization verifier: %v", err)
	}
	for name, mutate := range map[string]func(*CognitionProposalMaterialization){
		"reconciliation": func(value *CognitionProposalMaterialization) {
			value.ReconciliationID = "cognition_reconciliation_" + cognitionTestDigest("d")
		},
		"policy call": func(value *CognitionProposalMaterialization) {
			value.PolicyCallID = "cognition_call_" + cognitionTestDigest("e")
		},
		"call ordinal": func(value *CognitionProposalMaterialization) { value.CallOrdinal++ },
	} {
		t.Run(name, func(t *testing.T) {
			forged := value
			mutate(&forged)
			forged.SHA256, err = cognitionProposalCanonicalSHA(forged.identity())
			if err != nil {
				t.Fatal(err)
			}
			forged.ID = cognitionProposalMaterializationPrefix + forged.SHA256
			if err := forged.Validate(); err != nil {
				t.Fatalf("coherent lifecycle forgery is not independently valid: %v", err)
			}
			if err := VerifyCognitionProposalMaterializationTrace(
				forged, binding, snapshot, command.Decision, command.ActionSchema,
			); err == nil {
				t.Fatal("portable verifier accepted a changed lifecycle tuple")
			}
		})
	}
	forged := value
	forged.PreProposalLedger.Entries = append(
		[]taskstate.Entry(nil), value.PreProposalLedger.Entries...,
	)
	forged.PreProposalLedger.Entries[0].UpdatedVersion = forged.PreProposalLedger.Version + 1
	if err := VerifyCognitionProposalMaterializationTrace(
		forged, binding, snapshot, command.Decision, command.ActionSchema,
	); err == nil {
		t.Fatal("portable verifier accepted an embedded ledger entry newer than its bound version")
	}
}
