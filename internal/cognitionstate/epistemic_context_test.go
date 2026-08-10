package cognitionstate

import (
	"testing"

	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func TestReconciliationProjectsOnlyCurrentEpistemicStateWithDistinctRoles(t *testing.T) {
	t.Parallel()
	snapshot, graph, observation := attentionTestRuntime(t)
	ledger, err := taskstate.RestoreLedger(attentionTestLedger(t, observation))
	if err != nil {
		t.Fatal(err)
	}
	for _, nodeID := range []taskstate.NodeID{"obligation-41", "obligation-ready"} {
		applyEpistemicCommand(t, ledger, "node-"+string(nodeID), taskstate.AddNodeCommand{
			Actor: taskstate.AuthorityCode, ID: nodeID, Kind: taskstate.NodeGoal,
			Title: string(nodeID), Priority: 100, Metadata: taskstate.EmptyJSONObject(),
		})
	}
	evidence := evidenceLedgerRef(observation.EvidenceRef())
	applyEpistemicCommand(t, ledger, "fact-current", taskstate.AddEntryCommand{
		Actor: taskstate.AuthorityCode, ID: "fact-current", ScopeNodeID: "obligation-41",
		Kind: taskstate.EntryFact, Content: "Accepted current fact.",
		Metadata: taskstate.EmptyJSONObject(), Refs: []taskstate.Ref{evidence},
	})
	applyEpistemicCommand(t, ledger, "hypothesis-current", taskstate.AddEntryCommand{
		Actor: taskstate.AuthorityModelProposal, ID: "hypothesis-current", ScopeNodeID: "obligation-41",
		Kind: taskstate.EntryHypothesis, Content: "Unresolved current hypothesis.",
		Metadata: taskstate.EmptyJSONObject(), Refs: []taskstate.Ref{},
	})
	applyEpistemicCommand(t, ledger, "question-current", taskstate.AddEntryCommand{
		Actor: taskstate.AuthorityModelProposal, ID: "question-current", ScopeNodeID: "obligation-41",
		Kind: taskstate.EntryQuestion, Content: "Unresolved current question?",
		Metadata: taskstate.EmptyJSONObject(), Refs: []taskstate.Ref{},
	})
	applyEpistemicCommand(t, ledger, "decision-candidate", taskstate.AddEntryCommand{
		Actor: taskstate.AuthorityModelProposal, ID: "decision-candidate", ScopeNodeID: "obligation-41",
		Kind: taskstate.EntryDecisionCandidate, Content: "Accepted current decision.",
		Metadata: taskstate.EmptyJSONObject(), Refs: []taskstate.Ref{},
	})
	applyEpistemicCommand(t, ledger, "decision-accept", taskstate.AcceptDecisionCommand{
		Actor: taskstate.AuthorityCode, CandidateID: "decision-candidate", AcceptedEntryID: "decision-current",
		AcceptancePolicy: "code_verified_cognition_decision_v1", AcceptanceRefs: []taskstate.Ref{evidence},
		Metadata: taskstate.EmptyJSONObject(),
	})
	for index, kind := range []taskstate.EntryKind{
		taskstate.EntryFact, taskstate.EntryHypothesis, taskstate.EntryQuestion, taskstate.EntryDecisionCandidate,
	} {
		refs := []taskstate.Ref{}
		if kind == taskstate.EntryFact {
			refs = []taskstate.Ref{evidence}
		}
		applyEpistemicCommand(t, ledger, "sibling-entry-"+string(rune('a'+index)), taskstate.AddEntryCommand{
			Actor: epistemicActor(kind), ID: taskstate.EntryID("sibling-entry-" + string(rune('a'+index))),
			ScopeNodeID: "obligation-ready", Kind: kind, Content: "Future sibling epistemic state.",
			Metadata: taskstate.EmptyJSONObject(), Refs: refs,
		})
	}
	applyEpistemicCommand(t, ledger, "global-fact", taskstate.AddEntryCommand{
		Actor: taskstate.AuthorityCode, ID: "global-fact", Kind: taskstate.EntryFact,
		Content: "Unscoped fact is not automatically relevant.", Metadata: taskstate.EmptyJSONObject(),
		Refs: []taskstate.Ref{evidence},
	})

	state := ledger.MaterializedState()
	set := attentionTestWorkingSet(t, state, workingset.Budget{
		MaxItems: 32, MaxBytes: 256 * 1024, MaxPinnedItems: 24, MaxPinnedBytes: 192 * 1024,
	})
	plan, err := BuildDefaultReconciliation(ReconciliationInput{
		State: attentionProjectionState(t, snapshot), ObligationGraph: graph,
		Ledger: state, WorkingSet: set.Snapshot(),
		Evidence: []EvidenceMaterial{{Ref: observation.EvidenceRef(), Content: observation.Content}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]workingset.Role{
		"Accepted current fact.":         workingset.RoleFact,
		"Unresolved current hypothesis.": workingset.RoleHypothesis,
		"Unresolved current question?":   workingset.RoleQuestion,
		"Accepted current decision.":     workingset.RoleDecision,
	}
	for _, material := range plan.Materials() {
		role, exists := want[material.Content]
		if !exists {
			if material.Content == "Future sibling epistemic state." ||
				material.Content == "Unscoped fact is not automatically relevant." {
				t.Fatalf("clean desk projected unrelated epistemic state %q", material.Content)
			}
			continue
		}
		item, exists := applyAttentionPlan(t, set.Snapshot(), plan).Item(material.ItemID)
		if !exists || item.Role != role || item.Retention != workingset.RetentionPinned {
			t.Fatalf("epistemic material %q item = %#v, want role %q pinned", material.Content, item, role)
		}
		delete(want, material.Content)
	}
	if len(want) != 0 {
		t.Fatalf("missing epistemic projections: %#v", want)
	}
}

func applyEpistemicCommand(t *testing.T, ledger *taskstate.Ledger, identity string, command taskstate.Command) {
	t.Helper()
	commandID, err := taskstate.NewCommandID(t.Name(), identity)
	if err != nil {
		t.Fatal(err)
	}
	switch typed := command.(type) {
	case taskstate.AddNodeCommand:
		typed.CommandID, typed.ExpectedVersion = commandID, ledger.Version()
		_, err = ledger.Apply(typed)
	case taskstate.AddEntryCommand:
		typed.CommandID, typed.ExpectedVersion = commandID, ledger.Version()
		_, err = ledger.Apply(typed)
	case taskstate.AcceptDecisionCommand:
		typed.CommandID, typed.ExpectedVersion = commandID, ledger.Version()
		_, err = ledger.Apply(typed)
	default:
		t.Fatalf("unsupported epistemic command %T", command)
	}
	if err != nil {
		t.Fatalf("apply %s: %v", identity, err)
	}
}

func epistemicActor(kind taskstate.EntryKind) taskstate.Authority {
	if kind == taskstate.EntryFact {
		return taskstate.AuthorityCode
	}
	return taskstate.AuthorityModelProposal
}
