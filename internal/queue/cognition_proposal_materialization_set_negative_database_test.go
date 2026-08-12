package queue

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/taskstate"
)

func TestPostgresCognitionProposalMaterializationPortableSetRejectsIncompleteAuthority(t *testing.T) {
	fixture := newCognitionProposalMaterializationSetFixture(t)
	tests := map[string]func() ([]CognitionProposalMaterializationTraceMember,
		CognitionProposalMaterializationReconciliationAuthority,
		cognition.RuntimeSnapshot, cognitionruntime.ReconciliationCommand,
		cognitionruntime.ReconciliationReceipt){
		"duplicate index": func() ([]CognitionProposalMaterializationTraceMember,
			CognitionProposalMaterializationReconciliationAuthority, cognition.RuntimeSnapshot,
			cognitionruntime.ReconciliationCommand, cognitionruntime.ReconciliationReceipt) {
			return []CognitionProposalMaterializationTraceMember{
				fixture.Members[0], fixture.Members[0],
			}, fixture.Authority, fixture.Snapshot, fixture.Command, fixture.Receipt
		},
		"reordered": func() ([]CognitionProposalMaterializationTraceMember,
			CognitionProposalMaterializationReconciliationAuthority, cognition.RuntimeSnapshot,
			cognitionruntime.ReconciliationCommand, cognitionruntime.ReconciliationReceipt) {
			return []CognitionProposalMaterializationTraceMember{
				fixture.Members[1], fixture.Members[0],
			}, fixture.Authority, fixture.Snapshot, fixture.Command, fixture.Receipt
		},
		"changed accepted call": func() ([]CognitionProposalMaterializationTraceMember,
			CognitionProposalMaterializationReconciliationAuthority, cognition.RuntimeSnapshot,
			cognitionruntime.ReconciliationCommand, cognitionruntime.ReconciliationReceipt) {
			members := rebindCognitionProposalMembers(
				t, fixture.Members, fixture.Receipt.ID,
				"cognition_call_"+cognitionTestDigest("9"), fixture.Authority.CallOrdinal+1,
			)
			return members, fixture.Authority, fixture.Snapshot, fixture.Command, fixture.Receipt
		},
		"changed final receipt": func() ([]CognitionProposalMaterializationTraceMember,
			CognitionProposalMaterializationReconciliationAuthority, cognition.RuntimeSnapshot,
			cognitionruntime.ReconciliationCommand, cognitionruntime.ReconciliationReceipt) {
			receipt, err := cognitionruntime.NewReconciliationReceipt(
				fixture.Command, fixture.Receipt.LedgerVersion+1, fixture.Receipt.WorkingSetVersion,
			)
			if err != nil {
				t.Fatal(err)
			}
			authority := fixture.Authority
			authority.ReconciliationID = receipt.ID
			members := rebindCognitionProposalMembers(
				t, fixture.Members, receipt.ID, authority.PolicyCallID, authority.CallOrdinal,
			)
			return members, authority, fixture.Snapshot, fixture.Command, receipt
		},
	}
	for name, build := range tests {
		t.Run(name, func(t *testing.T) {
			members, authority, snapshot, command, receipt := build()
			if err := VerifyCognitionProposalMaterializationTraceSet(
				members, authority, snapshot, command, receipt,
			); err == nil {
				t.Fatal("portable set verifier accepted incomplete or changed authority")
			}
		})
	}
}

func TestPostgresCognitionProposalMaterializationPortableSetRejectsMixedLedgerAndSnapshot(t *testing.T) {
	fixture := newCognitionProposalMaterializationSetFixture(t)
	ledger := reorderedCognitionProposalLedger(t, fixture.Members[0].Value.PreProposalLedger)
	ledgerValues, err := newCognitionProposalMaterializations(
		fixture.Members[0].Value.EpisodeID, fixture.Authority.PolicyCallID,
		fixture.Authority.CallOrdinal, cognitionstate.ModelProposalInput{
			Ledger: ledger, ScopeNodeID: taskstate.NodeID(fixture.Command.Decision.ObligationID),
			Snapshot: fixture.Snapshot, Decision: fixture.Command.Decision,
			ActionSchema: fixture.Command.ActionSchema,
		}, fixture.Receipt,
	)
	if err != nil {
		t.Fatal(err)
	}
	mixedLedger := append([]CognitionProposalMaterializationTraceMember(nil), fixture.Members...)
	mixedLedger[1] = cognitionProposalTraceMembers(t, ledgerValues)[1]
	if err := VerifyCognitionProposalMaterializationTraceSet(
		mixedLedger, fixture.Authority, fixture.Snapshot, fixture.Command, fixture.Receipt,
	); err == nil {
		t.Fatal("portable set verifier accepted independently valid mixed pre-proposal ledgers")
	}

	budget := fixture.Snapshot.Budget()
	budget.RemainingPolicyCalls--
	alternateSnapshot, err := cognition.NewRuntimeSnapshot(
		fixture.Snapshot.Goal(), fixture.Snapshot.CurrentRevision(),
		fixture.Snapshot.CurrentObligation(), fixture.Snapshot.ActionCatalog(),
		fixture.Snapshot.Attempt(), fixture.Snapshot.ContextProjection(), budget,
		fixture.Snapshot.EvidenceRefs(),
	)
	if err != nil {
		t.Fatal(err)
	}
	alternateCommand := fixture.Command.Clone()
	alternateCommand.SnapshotSHA256 = alternateSnapshot.SHA256()
	alternateReceipt, err := cognitionruntime.NewReconciliationReceipt(
		alternateCommand, fixture.Receipt.LedgerVersion, fixture.Receipt.WorkingSetVersion,
	)
	if err != nil {
		t.Fatal(err)
	}
	alternateAuthority := CognitionProposalMaterializationReconciliationAuthority{
		ReconciliationID: alternateReceipt.ID,
		PolicyCallID:     "cognition_call_" + cognitionTestDigest("8"),
		CallOrdinal:      fixture.Authority.CallOrdinal + 1,
	}
	alternateValues, err := newCognitionProposalMaterializations(
		fixture.Members[0].Value.EpisodeID, alternateAuthority.PolicyCallID,
		alternateAuthority.CallOrdinal, cognitionstate.ModelProposalInput{
			Ledger:      fixture.Members[0].Value.PreProposalLedger,
			ScopeNodeID: taskstate.NodeID(alternateCommand.Decision.ObligationID),
			Snapshot:    alternateSnapshot, Decision: alternateCommand.Decision,
			ActionSchema: alternateCommand.ActionSchema,
		}, alternateReceipt,
	)
	if err != nil {
		t.Fatal(err)
	}
	alternateMembers := cognitionProposalTraceMembers(t, alternateValues)
	if err := VerifyCognitionProposalMaterializationTraceSet(
		alternateMembers, alternateAuthority, alternateSnapshot, alternateCommand, alternateReceipt,
	); err != nil {
		t.Fatalf("independently valid alternate snapshot set: %v", err)
	}
	mixedSnapshot := append([]CognitionProposalMaterializationTraceMember(nil), fixture.Members...)
	mixedSnapshot[1] = alternateMembers[1]
	if err := VerifyCognitionProposalMaterializationTraceSet(
		mixedSnapshot, fixture.Authority, fixture.Snapshot, fixture.Command, fixture.Receipt,
	); err == nil {
		t.Fatal("portable set verifier accepted mixed lifecycle and snapshot tuples")
	}
}

func TestPostgresCognitionProposalMaterializationPortableSetRejectsRevisionMember(t *testing.T) {
	fixture := newCognitionProposalMaterializationSetFixture(t)
	command, receipt := cognitionProposalRevisionCommand(t, fixture)
	authority := fixture.Authority
	authority.ReconciliationID = receipt.ID
	if err := VerifyCognitionProposalMaterializationTraceSet(
		nil, authority, fixture.Snapshot, command, receipt,
	); err != nil {
		t.Fatalf("revision-only reconciliation must have an empty materialization set: %v", err)
	}
	if err := VerifyCognitionProposalMaterializationTraceSet(
		fixture.Members, authority, fixture.Snapshot, command, receipt,
	); err == nil {
		t.Fatal("revision-only reconciliation accepted proposal materialization members")
	}
}

func rebindCognitionProposalMembers(
	t *testing.T, source []CognitionProposalMaterializationTraceMember,
	reconciliationID, policyCallID string, callOrdinal uint64,
) []CognitionProposalMaterializationTraceMember {
	t.Helper()
	values := make([]CognitionProposalMaterialization, len(source))
	for index, member := range source {
		value := member.Value
		value.ReconciliationID, value.PolicyCallID, value.CallOrdinal =
			reconciliationID, policyCallID, callOrdinal
		var err error
		value.SHA256, err = cognitionProposalCanonicalSHA(value.identity())
		if err != nil {
			t.Fatal(err)
		}
		value.ID = cognitionProposalMaterializationPrefix + value.SHA256
		values[index] = value
	}
	return cognitionProposalTraceMembers(t, values)
}

func cognitionProposalTraceMembers(
	t *testing.T, values []CognitionProposalMaterialization,
) []CognitionProposalMaterializationTraceMember {
	t.Helper()
	members := make([]CognitionProposalMaterializationTraceMember, len(values))
	for index, value := range values {
		raw, err := exactjson.Canonical(value)
		if err != nil {
			t.Fatal(err)
		}
		members[index] = CognitionProposalMaterializationTraceMember{
			Value: value,
			Authority: CognitionProposalMaterializationTraceAuthority{
				ReconciliationID: value.ReconciliationID, PolicyCallID: value.PolicyCallID,
				CallOrdinal: value.CallOrdinal, Phase: CognitionProposalMaterializationTracePhase,
				Sequence: int64(value.ProposalIndex), ID: value.ID, SHA256: cognitionPayloadSHA(raw),
			},
		}
	}
	return members
}

func reorderedCognitionProposalLedger(
	t *testing.T, source taskstate.MaterializedState,
) taskstate.MaterializedState {
	t.Helper()
	raw, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	var value taskstate.MaterializedState
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	swapped := false
	if len(value.Nodes) > 1 {
		value.Nodes[0], value.Nodes[1], swapped = value.Nodes[1], value.Nodes[0], true
	} else if len(value.Entries) > 1 {
		value.Entries[0], value.Entries[1], swapped = value.Entries[1], value.Entries[0], true
	} else if len(value.Edges) > 1 {
		value.Edges[0], value.Edges[1], swapped = value.Edges[1], value.Edges[0], true
	}
	if !swapped || taskstate.ValidateMaterializedState(value) != nil {
		t.Fatal("fixture lacks a second record for a valid reordered ledger")
	}
	return value
}
