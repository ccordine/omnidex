package host

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/labyrinth"
)

func TestPostgresCompletionEvaluatorUsesExactDurableWorldTruth(t *testing.T) {
	fixture := newDurableFixture(t)
	environment := fixture.environment(t, func(actor cognition.AttemptRef) bool { return actor == fixture.Actor })
	started, err := environment.Start(context.Background(), fixture.Scenario.Ref())
	if err != nil {
		t.Fatal(err)
	}
	request := completionRequest(t, fixture, started, false)
	result, err := environment.Evaluate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != cognition.CompletionUnsatisfied || len(result.EvidenceRefs) != 0 {
		t.Fatalf("initial completion = %#v", result)
	}

	current := started
	observations := append([]cognition.Observation(nil), started.Observations...)
	for index := range fixture.Witness {
		witness := fixture.Witness[index]
		schema, exists := fixture.Scenario.Catalog().Schema(witness.Request.Kind)
		if !exists {
			t.Fatalf("witness schema %q is absent", witness.Request.Kind)
		}
		var evidence []cognition.EvidenceRef
		if schema.EvidencePolicy == cognition.EvidenceRequired {
			evidence = make([]cognition.EvidenceRef, len(observations))
			for position, observation := range observations {
				evidence[position] = observation.EvidenceRef()
			}
		}
		action, err := cognition.NewRegisteredAction(
			cognition.ActionID("completion-action-"+string(rune('a'+index))),
			fixture.Actor, schema, witness.Request, evidence,
		)
		if err != nil {
			t.Fatalf("register witness action %d: %v", index, err)
		}
		current, err = environment.Apply(context.Background(), fixture.Episode, current.Current, action)
		if err != nil {
			t.Fatalf("witness action %d: %v", index, err)
		}
		observations = append(observations, current.Observations...)
	}
	if !current.Terminal {
		t.Fatal("witness did not reach durable terminal state")
	}
	request = completionRequest(t, fixture, current, true)
	result, err = environment.Evaluate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != cognition.CompletionSatisfied || len(result.EvidenceRefs) != 1 ||
		result.EvidenceRefs[0] != current.Observations[0].EvidenceRef() {
		t.Fatalf("terminal completion = %#v", result)
	}
}

func TestPostgresCompletionEvaluatorUsesOneCheckForEachExactDesiredExpression(t *testing.T) {
	fixture := newDurableFixture(t)
	environment := fixture.environment(t, func(actor cognition.AttemptRef) bool { return actor == fixture.Actor })
	started, err := environment.Start(t.Context(), fixture.Scenario.Ref())
	if err != nil {
		t.Fatal(err)
	}
	root := completionRequest(t, fixture, started, false)
	rootResult, err := environment.Evaluate(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if rootResult.Outcome != cognition.CompletionUnsatisfied {
		t.Fatalf("root completion = %#v", rootResult)
	}

	negative := root
	negative.Obligation.ID = "labyrinth-negative-subgoal"
	negative.Obligation.Desired = cognition.GoalExpression{
		Not: []cognition.Predicate{root.Goal.All[0].Clone()},
	}
	negativeResult, err := environment.Evaluate(t.Context(), negative)
	if err != nil {
		t.Fatal(err)
	}
	if negative.Obligation.CompletionCheck != root.Obligation.CompletionCheck ||
		negativeResult.Outcome != cognition.CompletionSatisfied ||
		len(negativeResult.EvidenceRefs) != 1 ||
		negativeResult.EvidenceRefs[0] != started.Observations[0].EvidenceRef() {
		t.Fatalf("negative completion = %#v root check=%#v", negativeResult, root.Obligation.CompletionCheck)
	}
}

func TestPostgresCompletionEvaluatorRequiresExactTerminalObservationInPacket(t *testing.T) {
	fixture := newDurableFixture(t)
	environment := fixture.environment(t, func(actor cognition.AttemptRef) bool { return actor == fixture.Actor })
	current, err := environment.Start(t.Context(), fixture.Scenario.Ref())
	if err != nil {
		t.Fatal(err)
	}
	startEvidence := current.Observations[0].EvidenceRef()
	observations := append([]cognition.Observation(nil), current.Observations...)
	for index, witness := range fixture.Witness {
		schema, exists := fixture.Scenario.Catalog().Schema(witness.Request.Kind)
		if !exists {
			t.Fatalf("witness schema %q is absent", witness.Request.Kind)
		}
		var evidence []cognition.EvidenceRef
		if schema.EvidencePolicy == cognition.EvidenceRequired {
			for _, observation := range observations {
				evidence = append(evidence, observation.EvidenceRef())
			}
		}
		action, err := cognition.NewRegisteredAction(
			cognition.ActionID("missing-completion-evidence-"+string(rune('a'+index))),
			fixture.Actor, schema, witness.Request, evidence,
		)
		if err != nil {
			t.Fatal(err)
		}
		current, err = environment.Apply(t.Context(), fixture.Episode, current.Current, action)
		if err != nil {
			t.Fatal(err)
		}
		observations = append(observations, current.Observations...)
	}
	request := completionRequest(t, fixture, current, true)
	request.Obligation.SupportingRefs = []cognition.EvidenceRef{}
	request.EvidenceRefs = []cognition.EvidenceRef{startEvidence}
	if _, err := environment.Evaluate(t.Context(), request); !errors.Is(err, cognition.ErrInvalidEvidence) {
		t.Fatalf("missing terminal observation error=%v, want invalid evidence", err)
	}
}

func TestPostgresCompletionEvaluatorRejectsStaleAndChangedAuthority(t *testing.T) {
	fixture := newDurableFixture(t)
	environment := fixture.environment(t, func(actor cognition.AttemptRef) bool { return actor == fixture.Actor })
	started, err := environment.Start(context.Background(), fixture.Scenario.Ref())
	if err != nil {
		t.Fatal(err)
	}
	request := completionRequest(t, fixture, started, false)

	changedCheck := request
	changedCheck.Obligation.CompletionCheck.SHA256 = strings.Repeat("f", 64)
	if _, err := environment.Evaluate(context.Background(), changedCheck); !errors.Is(err, cognition.ErrInvalidCompletionCheck) {
		t.Fatalf("changed completion check error = %v", err)
	}
	staleEpisode := request
	staleEpisode.Binding.Episode.ID = "another-durable-episode"
	if _, err := environment.Evaluate(context.Background(), staleEpisode); !errors.Is(err, cognition.ErrInvalidRevision) {
		t.Fatalf("stale episode error = %v", err)
	}
	staleRevision := request
	staleRevision.Revision.SHA256 = strings.Repeat("e", 64)
	if _, err := environment.Evaluate(context.Background(), staleRevision); !errors.Is(err, cognition.ErrInvalidRevision) {
		t.Fatalf("stale revision error = %v", err)
	}
	staleActor := request
	staleActor.Binding.Attempt.WorkerID = "stale-worker"
	if _, err := environment.Evaluate(context.Background(), staleActor); !errors.Is(err, cognition.ErrAuthorityDenied) {
		t.Fatalf("stale worker error = %v", err)
	}
}

func completionRequest(
	t *testing.T,
	fixture durableFixture,
	transition cognition.Transition,
	terminal bool,
) cognitionruntime.CompletionRequest {
	t.Helper()
	goal := fixture.Scenario.Goal()
	check, err := labyrinth.NewCompletionCheck()
	if err != nil {
		t.Fatal(err)
	}
	evidence := transition.Observations[0].EvidenceRef()
	obligation := cognition.Obligation{
		ID: "labyrinth-root", Desired: goal, Status: cognition.ObligationActive,
		DependsOn: []cognition.ObligationID{}, SupportingRefs: []cognition.EvidenceRef{evidence},
		CompletionCheck: check, CreatedGeneration: 1,
	}
	publicOutcome := ""
	if terminal {
		publicOutcome = labyrinth.PublicOutcomeGoalSatisfied
	}
	return cognitionruntime.CompletionRequest{
		Binding:        cognitionruntime.Binding{Episode: fixture.Episode, Attempt: fixture.Actor},
		SnapshotSHA256: strings.Repeat("a", 64), Goal: goal, Revision: transition.Current,
		Obligation: obligation, EvidenceRefs: []cognition.EvidenceRef{evidence},
		EnvironmentTerminal: terminal, PublicOutcome: publicOutcome,
	}
}

func (fixture durableFixture) StoreEpisodeStartEvidence(t *testing.T) cognition.EvidenceRef {
	t.Helper()
	receipt, err := fixture.Store.Episode(context.Background(), fixture.Episode)
	if err != nil {
		t.Fatal(err)
	}
	return receipt.Start.Observations[0].EvidenceRef()
}
