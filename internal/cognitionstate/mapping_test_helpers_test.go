package cognitionstate

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
)

const mappingTestDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func mappingTestLedger(t *testing.T) taskstate.MaterializedState {
	t.Helper()
	owner := taskstate.LedgerOwner{
		Kind: taskstate.OwnerJob, JobID: 41,
		RunID: "01234567-89ab-cdef-0123-456789abcdef",
	}
	id, err := taskstate.NewLedgerID(owner)
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := taskstate.NewLedger(id, owner)
	if err != nil {
		t.Fatal(err)
	}
	return ledger.MaterializedState()
}

func mappingTestRevision() cognition.WorldRevision {
	return cognition.WorldRevision{EpisodeID: "episode-41", Number: 3, SHA256: mappingTestDigest}
}

func mappingPriorRevision() cognition.WorldRevision {
	return cognition.WorldRevision{
		EpisodeID: "episode-41", Number: 2,
		SHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
}

func mappingTestSchema(t *testing.T) cognition.ActionSchema {
	t.Helper()
	schema, err := cognition.NewActionSchema(
		"catalog.inspect.v1", "1.0.0", "inspect",
		[]cognition.ActionParameterSpec{{Name: "target", Required: true, MaxBytes: 128}},
		cognition.EvidenceOptional,
	)
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func mappingTestAction(t *testing.T, schema cognition.ActionSchema) cognition.RegisteredAction {
	t.Helper()
	request, err := cognition.NewActionRequest("inspect", []cognition.ActionArgument{{Name: "target", Value: "entity-1"}})
	if err != nil {
		t.Fatal(err)
	}
	action, err := cognition.NewRegisteredAction(
		"action-41",
		cognition.AttemptRef{JobID: 41, Generation: 2, StepID: 9, Attempt: 1, WorkerID: "worker-41"},
		schema, request, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	return action
}

func mappingTestObservation(t *testing.T, actionID cognition.ActionID) cognition.Observation {
	t.Helper()
	var (
		observation cognition.Observation
		err         error
	)
	if actionID == "" {
		observation, err = cognition.NewObservation(
			"observation-41", mappingTestRevision(), "record", "The public value is amber.",
		)
	} else {
		observation, err = cognition.NewActionObservation(
			"observation-41", actionID, mappingTestRevision(), "record", "The public value is amber.",
		)
	}
	if err != nil {
		t.Fatal(err)
	}
	return observation
}

func mappingTestSnapshot(t *testing.T, evidence cognition.EvidenceRef) cognition.RuntimeSnapshot {
	t.Helper()
	predicate, err := cognition.NewPredicate("goal.condition", []string{"target-41"})
	if err != nil {
		t.Fatal(err)
	}
	goal, err := cognition.NewGoalExpression([]cognition.Predicate{predicate}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	obligation := cognition.Obligation{
		ID: "obligation-41", Desired: goal, Status: cognition.ObligationActive,
		SupportingRefs: []cognition.EvidenceRef{evidence},
		CompletionCheck: cognition.CompletionCheckRef{
			ID: "check.goal", Version: "1.0.0", SHA256: mappingTestDigest,
		},
		CreatedGeneration: 2,
	}
	schema := mappingTestSchema(t)
	catalog, err := cognition.NewActionCatalog("catalog.mapping", "1.0.0", []cognition.ActionSchema{schema})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := cognition.NewRuntimeSnapshot(
		goal, mappingTestRevision(), obligation, catalog,
		cognition.AttemptRef{JobID: 41, Generation: 2, StepID: 9, Attempt: 1, WorkerID: "worker-41"},
		cognition.ContextProjectionRef{
			ID: "projection-41", SHA256: strings.Repeat("b", 64), WorkingSetID: "working-set-41",
			WorkingSetVersion: 1, RendererVersion: "omnidex.context-material-json.v1",
		},
		cognition.RuntimeBudget{
			RemainingPolicyCalls: 1, MaxInputBytes: 64 * 1024, MaxInputTokens: 16 * 1024,
			MaxOutputBytes: 16 * 1024, MaxOutputTokens: 4 * 1024,
			MaxEvidenceRefs: 4, MaxActionArguments: 4,
			MaxLedgerProposals: 4, MaxAttentionRequests: 4, MaxExpectedEffectBytes: 512,
		},
		[]cognition.EvidenceRef{evidence},
	)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
