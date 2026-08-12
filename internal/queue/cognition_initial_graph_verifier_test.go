package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/model"
)

func TestVerifyCognitionInitialObligationTraceAuthorityRejectsSubstitution(t *testing.T) {
	episode := cognition.EpisodeID("episode-initial-graph-verifier")
	actor := cognition.AttemptRef{
		JobID: 1, Generation: 2, StepID: 3, Attempt: 4, WorkerID: "worker-initial-graph",
	}
	goal := cognition.GoalExpression{All: []cognition.Predicate{{
		Name: "condition.initial", Args: []string{"target"},
	}}}
	check := cognition.CompletionCheckRef{
		ID: "check.initial", Version: "v1", SHA256: cognitionTestDigest("a"),
	}
	rootID, err := cognition.DeriveObligationID(
		episode, cognition.InitialObligationGeneration, "", goal, check,
	)
	if err != nil {
		t.Fatal(err)
	}
	root := cognition.ObligationSpec{
		ID: rootID, Desired: goal, DependsOn: []cognition.ObligationID{},
		SupportingRefs: []cognition.EvidenceRef{}, CompletionCheck: check,
	}
	want, descriptor, err := initialCognitionObligationGraph(CognitionEpisodeStart{
		Authority: model.StepAttemptAuthority{
			JobID: actor.JobID, Generation: actor.Generation, StepID: actor.StepID,
			Attempt: int64(actor.Attempt), WorkerID: actor.WorkerID,
		},
		EpisodeID: episode, Root: root,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyCognitionInitialObligationTraceAuthority(
		episode, actor, root, descriptor.ID, want,
	); err != nil {
		t.Fatal(err)
	}

	changedRoot := root
	changedRoot.CompletionCheck.ID = "check.changed"
	changedActor := actor
	changedActor.JobID++
	extraGoal := cognition.GoalExpression{All: []cognition.Predicate{{Name: "condition.extra"}}}
	extraID, err := cognition.DeriveObligationID(
		episode, cognition.InitialObligationGeneration, rootID, extraGoal, check,
	)
	if err != nil {
		t.Fatal(err)
	}
	extraGraph, err := cognition.NewObligationGraph(
		cognition.InitialObligationGeneration, rootID, []cognition.ObligationSpec{root, {
			ID: extraID, ParentID: rootID, Desired: extraGoal,
			DependsOn: []cognition.ObligationID{}, SupportingRefs: []cognition.EvidenceRef{},
			CompletionCheck: check,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := extraGraph.RefreshReadiness(cognition.InitialObligationGeneration); err != nil {
		t.Fatal(err)
	}
	if err := extraGraph.Transition(
		rootID, cognition.InitialObligationGeneration, cognition.ObligationActive,
	); err != nil {
		t.Fatal(err)
	}
	for name, verify := range map[string]func() error{
		"record_id": func() error {
			return VerifyCognitionInitialObligationTraceAuthority(
				episode, actor, root, descriptor.ID+"-changed", want,
			)
		},
		"root_contract": func() error {
			return VerifyCognitionInitialObligationTraceAuthority(
				episode, actor, changedRoot, descriptor.ID, want,
			)
		},
		"actor": func() error {
			return VerifyCognitionInitialObligationTraceAuthority(
				episode, changedActor, root, descriptor.ID, want,
			)
		},
		"extra_obligation": func() error {
			return VerifyCognitionInitialObligationTraceAuthority(
				episode, actor, root, descriptor.ID, extraGraph.Snapshot(),
			)
		},
	} {
		t.Run(name, func(t *testing.T) {
			if verify() == nil {
				t.Fatal("initial graph verifier accepted substituted authority")
			}
		})
	}
}
