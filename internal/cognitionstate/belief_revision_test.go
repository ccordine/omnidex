package cognitionstate

import (
	"errors"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
)

func TestBeliefRevisionMaterializesOneEvidenceBoundCodeRejection(t *testing.T) {
	t.Parallel()
	input := beliefRevisionTestInput(t)
	materialization, found, err := PlanBeliefRevision(input)
	if err != nil || !found {
		t.Fatalf("plan revision: found=%v error=%v", found, err)
	}
	if err := materialization.Validate(); err != nil {
		t.Fatalf("validate revision: %v", err)
	}
	if materialization.Rejection.Actor != taskstate.AuthorityCode ||
		materialization.Rejection.EntryID != "hypothesis-current" ||
		len(materialization.Rejection.Refs) != 1 ||
		materialization.Rejection.Refs[0].Relation != taskstate.RefContradicts {
		t.Fatalf("revision rejection = %#v", materialization.Rejection)
	}
	if mutations, err := MapModelProposals(input); err != nil || len(mutations) != 0 {
		t.Fatalf("revision created ordinary model entries: count=%d error=%v", len(mutations), err)
	}

	result, err := ApplyBeliefRevision(input.Ledger, materialization)
	if err != nil {
		t.Fatalf("apply revision: %v", err)
	}
	entry := revisionEntry(t, result, "hypothesis-current")
	if entry.Status != taskstate.EntryRejected || entry.DispositionBy != taskstate.AuthorityCode ||
		len(entry.Refs) != 1 || entry.Refs[0] != materialization.Rejection.Refs[0] {
		t.Fatalf("revised hypothesis = %#v", entry)
	}
	if _, err := ApplyBeliefRevision(result, materialization); !errors.Is(err, ErrInvalidMapping) {
		t.Fatalf("stale replay error = %v, want ErrInvalidMapping", err)
	}
	repeated, found, err := PlanBeliefRevision(input)
	if err != nil || !found || !reflect.DeepEqual(materialization, repeated) {
		t.Fatalf("revision planning is not deterministic: found=%v error=%v", found, err)
	}

	tampered := materialization
	tampered.Rejection.EntryID = "hypothesis-other"
	if err := tampered.Validate(); !errors.Is(err, ErrInvalidMapping) {
		t.Fatalf("tampered target error = %v, want ErrInvalidMapping", err)
	}
}

func TestBeliefRevisionRejectsUnseenOrNonHypothesisTargetsAndMissingToolEvidence(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*ModelProposalInput){
		"changed target hash": func(input *ModelProposalInput) {
			input.Decision.Proposals[0].Revision.TargetRef.SHA256 =
				"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		},
		"other obligation": func(input *ModelProposalInput) {
			input.Ledger.Entries[1].ScopeNodeID = "obligation-other"
		},
		"not a hypothesis": func(input *ModelProposalInput) {
			input.Ledger.Entries[1].Kind = taskstate.EntryQuestion
		},
		"not model authority": func(input *ModelProposalInput) {
			input.Ledger.Entries[1].Authority = taskstate.AuthorityCode
			input.Ledger.Entries[1].CreatedBy = taskstate.AuthorityCode
		},
		"missing tool evidence": func(input *ModelProposalInput) {
			input.Ledger.Entries[0].Status = taskstate.EntryRejected
			input.Ledger.Entries[0].DispositionReason = "Evidence was withdrawn."
			input.Ledger.Entries[0].DispositionBy = taskstate.AuthorityCode
		},
	} {
		mutate := mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			input := beliefRevisionTestInput(t)
			mutate(&input)
			if _, found, err := PlanBeliefRevision(input); err == nil || found {
				t.Fatalf("invalid revision found=%v error=%v", found, err)
			}
		})
	}
}

func beliefRevisionTestInput(t *testing.T) ModelProposalInput {
	t.Helper()
	observation := mappingTestObservation(t, "")
	evidence := observation.EvidenceRef()
	ledger, err := taskstate.RestoreLedger(mappingTestLedger(t))
	if err != nil {
		t.Fatal(err)
	}
	applyEpistemicCommand(t, ledger, "revision-node", taskstate.AddNodeCommand{
		Actor: taskstate.AuthorityCode, ID: "obligation-41", Kind: taskstate.NodeGoal,
		Title: "Current obligation", Priority: 100, Metadata: taskstate.EmptyJSONObject(),
	})
	observationMutation, err := MapEnvironmentObservation(EnvironmentObservationInput{
		Ledger: ledger.MaterializedState(), Observation: observation,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.Apply(observationMutation.Command()); err != nil {
		t.Fatal(err)
	}
	applyEpistemicCommand(t, ledger, "revision-hypothesis", taskstate.AddEntryCommand{
		Actor: taskstate.AuthorityModelProposal, ID: "hypothesis-current",
		ScopeNodeID: "obligation-41", Kind: taskstate.EntryHypothesis,
		Content: "The first mechanism remains available.", Metadata: taskstate.EmptyJSONObject(),
		Refs: []taskstate.Ref{},
	})
	state := ledger.MaterializedState()
	target := epistemicRef(state.ID, revisionEntry(t, state, "hypothesis-current"))
	snapshot := mappingTestSnapshot(t, evidence)
	schema := mappingTestSchema(t)
	decision := cognition.CognitionDecision{
		ObligationID: snapshot.CurrentObligation().ID,
		Action: cognition.ActionRequest{
			Kind: "inspect", Arguments: []cognition.ActionArgument{{Name: "target", Value: "entity-1"}},
		},
		EvidenceRefs: []cognition.EvidenceRef{evidence}, ExpectedEffect: "Inspect the current public state.",
		Proposals: []cognition.LedgerProposal{{
			Kind: cognition.ProposalRevision,
			Revision: &cognition.BeliefRevisionProposal{
				TargetRef: target, EvidenceRefs: []cognition.EvidenceRef{evidence},
			},
		}},
	}
	return ModelProposalInput{
		Ledger: state, ScopeNodeID: "obligation-41", Snapshot: snapshot,
		Decision: decision, ActionSchema: schema,
	}
}

func revisionEntry(t *testing.T, state taskstate.MaterializedState, id taskstate.EntryID) taskstate.Entry {
	t.Helper()
	for _, entry := range state.Entries {
		if entry.ID == id {
			return entry
		}
	}
	t.Fatalf("entry %q is missing", id)
	return taskstate.Entry{}
}
