package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/cognitionstate"
)

func TestPostgresCognitionAcceptedFactMaterializationBindsActionCallAndTracePhase(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "064")); err != nil {
		t.Fatal(err)
	}
	database := newCognitionDatabaseFixture(t, repository)
	facts := cognitionFactAuthorityForTest(t, func(
		transition cognition.Transition,
	) ([]cognitionstate.FactPlan, error) {
		if transition.Current.Number == 1 {
			return []cognitionstate.FactPlan{}, nil
		}
		return planFirstCognitionObservation(transition)
	}, cognitionTestDigest("7"))
	if _, err := repository.StartCognitionEpisode(t.Context(), database.Start, facts); err != nil {
		t.Fatal(err)
	}
	bound := buildCognitionDecisionStep(
		t, database, cognitionProposalMaterializationDecision(database),
	)
	receipt, err := repository.ReconcileCognitionRuntimeDecision(t.Context(), bound.Command)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := repository.PrepareCognitionAction(
		t.Context(), cognitionruntime.PrepareActionCommand{
			Binding: bound.Command.Binding, Coordinator: bound.Step, Reconciliation: receipt,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	dispatched, err := repository.DispatchCognitionAction(
		t.Context(), database.Authority, prepared.Action.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	next, err := cognition.NewWorldRevision(database.EpisodeID, 2, cognitionTestDigest("8"))
	if err != nil {
		t.Fatal(err)
	}
	observation, err := cognition.NewActionObservation(
		"accepted-fact-action-observation", dispatched.Action.ID, next,
		"public_state", "A later bounded public fact is visible.",
	)
	if err != nil {
		t.Fatal(err)
	}
	transition := cognition.Transition{
		ActionID: dispatched.Action.ID,
		Previous: cognitionRevisionPointer(dispatched.ExpectedRevision), Current: next,
		Observations: []cognition.Observation{observation}, Effects: []cognition.Effect{},
	}
	if _, err := repository.IngestCognitionTransition(
		t.Context(), database.Authority, dispatched.Action.ID, transition, facts,
	); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	var payloadSHA string
	if err := pool.QueryRow(t.Context(), `
		SELECT payload_json,payload_json_sha256
		FROM cognition_accepted_fact_materializations
		WHERE episode_id=$1 AND transition_revision=2
	`, database.EpisodeID).Scan(&raw, &payloadSHA); err != nil {
		t.Fatal(err)
	}
	value, err := DecodeCognitionAcceptedFactMaterialization(raw, payloadSHA)
	if err != nil {
		t.Fatal(err)
	}
	if value.ActionID != dispatched.Action.ID || value.CallOrdinal != bound.Prepared.CallOrdinal ||
		len(value.Members) != 1 {
		t.Fatalf("action accepted-fact tuple=%+v", value)
	}
	trace := CognitionAcceptedFactMaterializationTraceAuthority{
		TransitionID: value.TransitionID, TransitionSHA256: value.TransitionSHA256,
		CallOrdinal: value.CallOrdinal, Phase: CognitionAcceptedFactMaterializationActionTracePhase,
		Sequence: int64(value.TransitionRevision), ID: value.ID, SHA256: payloadSHA,
	}
	if err := VerifyCognitionAcceptedFactMaterializationTrace(
		value, trace, transition, facts,
	); err != nil {
		t.Fatalf("verify action accepted-fact materialization: %v", err)
	}
}
