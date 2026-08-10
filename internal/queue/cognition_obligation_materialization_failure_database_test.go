package queue

import (
	"errors"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/taskstate"
)

func TestPostgresCognitionMaterializationReconciliationReplayIsExact(t *testing.T) {
	_, repository, pool := openWorkingSetDatabase(t)
	fixture := newCognitionDatabaseFixture(t, repository)
	if _, err := repository.StartCognitionEpisode(t.Context(), fixture.Start, cognitionTestFactAuthority()); err != nil {
		t.Fatal(err)
	}
	proposal := buildCognitionProposalStep(t, fixture, "prerequisite")
	first, err := repository.ReconcileCognitionRuntimeDecision(t.Context(), proposal.Command)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := repository.ReconcileCognitionRuntimeDecision(t.Context(), proposal.Command.Clone())
	if err != nil || !reflect.DeepEqual(replay, first) {
		t.Fatalf("reconciliation replay=%+v error=%v, want %+v", replay, err, first)
	}
	changed := proposal.Command.Clone()
	changed.Decision.ExpectedEffect = "A changed expected effect must conflict."
	if _, err := repository.ReconcileCognitionRuntimeDecision(
		t.Context(), changed,
	); !errors.Is(err, ErrCognitionConflict) {
		t.Fatalf("changed reconciliation error=%v, want ErrCognitionConflict", err)
	}
	var reconciliations, materializations int
	if err := pool.QueryRow(t.Context(), `
		SELECT (SELECT COUNT(*) FROM cognition_reconciliations WHERE episode_id=$1),
		       (SELECT COUNT(*) FROM cognition_obligation_materializations WHERE episode_id=$1)
	`, fixture.EpisodeID).Scan(&reconciliations, &materializations); err != nil {
		t.Fatal(err)
	}
	if reconciliations != 1 || materializations != 1 {
		t.Fatalf("reconciliation/materialization counts=%d/%d", reconciliations, materializations)
	}
}

func TestPostgresCognitionRejectsUnsupportedObligationBeforeReconciliation(t *testing.T) {
	_, repository, pool := openWorkingSetDatabase(t)
	fixture := newCognitionDatabaseFixture(t, repository)
	if _, err := repository.StartCognitionEpisode(t.Context(), fixture.Start, cognitionTestFactAuthority()); err != nil {
		t.Fatal(err)
	}
	proposal := buildCognitionProposalStep(t, fixture, "unsupported")
	if _, err := repository.ReconcileCognitionRuntimeDecision(
		t.Context(), proposal.Command,
	); !errors.Is(err, cognition.ErrUnsupportedCompletionPredicate) {
		t.Fatalf("unsupported materialization error=%v", err)
	}
	var reconciliations, materializations int
	if err := pool.QueryRow(t.Context(), `
		SELECT (SELECT COUNT(*) FROM cognition_reconciliations WHERE episode_id=$1),
		       (SELECT COUNT(*) FROM cognition_obligation_materializations WHERE episode_id=$1)
	`, fixture.EpisodeID).Scan(&reconciliations, &materializations); err != nil {
		t.Fatal(err)
	}
	if reconciliations != 0 || materializations != 0 {
		t.Fatalf("unsupported proposal persisted reconciliation/materialization=%d/%d",
			reconciliations, materializations)
	}
}

func TestPostgresCognitionRejectsMaterializationAfterGraphAdvances(t *testing.T) {
	_, repository, pool := openWorkingSetDatabase(t)
	fixture := newCognitionDatabaseFixture(t, repository)
	fixture.Start.Transition.Terminal = true
	fixture.Start.Transition.PublicOutcome = "The public environment ended before action execution."
	if _, err := repository.StartCognitionEpisode(t.Context(), fixture.Start, cognitionTestFactAuthority()); err != nil {
		t.Fatal(err)
	}
	proposal := buildCognitionProposalStep(t, fixture, "prerequisite")
	receipt, err := repository.ReconcileCognitionRuntimeDecision(t.Context(), proposal.Command)
	if err != nil {
		t.Fatal(err)
	}
	completion, err := cognition.NewCompletionResult(
		fixture.Start.Root.ID, fixture.Check, fixture.Start.Transition.Current,
		cognition.CompletionUnsatisfied, []cognition.EvidenceRef{},
	)
	if err != nil {
		t.Fatal(err)
	}
	progress, err := repository.FailCognitionRuntimeTerminal(
		t.Context(), cognitionruntime.CompletionCommand{
			Binding:         proposal.Command.Binding,
			SnapshotSHA256:  proposal.Prepared.Prepared.Snapshot.SHA256(),
			GraphVersion:    proposal.Prepared.Prepared.GraphVersion,
			ObligationGraph: proposal.Prepared.Prepared.ObligationGraph,
			CompletionEvidenceRefs: append(
				[]cognition.EvidenceRef{}, proposal.Prepared.Prepared.CompletionEvidenceRefs...,
			),
			Result: completion, EnvironmentTerminal: true,
			PublicOutcome: fixture.Start.Transition.PublicOutcome,
		},
	)
	if err != nil || progress.State != cognitionruntime.ProgressFailed {
		t.Fatalf("terminal graph advance=%+v error=%v", progress, err)
	}
	if _, err := repository.PrepareCognitionAction(
		t.Context(), cognitionruntime.PrepareActionCommand{
			Binding: proposal.Command.Binding, Coordinator: proposal.Step,
			Reconciliation: receipt,
		},
	); !errors.Is(err, ErrCognitionConflict) {
		t.Fatalf("stale materialization preparation error=%v, want ErrCognitionConflict", err)
	}
	var actions, applications int
	if err := pool.QueryRow(t.Context(), `
		SELECT (SELECT COUNT(*) FROM cognition_actions WHERE episode_id=$1),
		       (SELECT COUNT(*) FROM cognition_obligation_materialization_applications WHERE episode_id=$1)
	`, fixture.EpisodeID).Scan(&actions, &applications); err != nil {
		t.Fatal(err)
	}
	if actions != 0 || applications != 0 {
		t.Fatalf("stale materialization actions/applications=%d/%d", actions, applications)
	}
}

func TestPostgresCognitionMaterializationFailureRollsBackWholeTransition(t *testing.T) {
	_, repository, pool := openWorkingSetDatabase(t)
	fixture := newCognitionDatabaseFixture(t, repository)
	if _, err := repository.StartCognitionEpisode(t.Context(), fixture.Start, cognitionTestFactAuthority()); err != nil {
		t.Fatal(err)
	}
	reserveMaterializationChildTaskNode(t, fixture)
	action, _ := prepareCognitionProposalAction(t, fixture)
	transition := cognitionProposalTransition(t, fixture, action)
	if _, err := repository.IngestCognitionTransition(
		t.Context(), fixture.Authority, action.Action.ID, transition, cognitionTestFactAuthority(),
	); err == nil {
		t.Fatal("materialization collision unexpectedly committed")
	}
	var status CognitionActionStatus
	var revision, graphVersion, transitions, applications int64
	if err := pool.QueryRow(t.Context(), `
		SELECT actions.status,episodes.current_revision,
		       (SELECT MAX(graph_version) FROM cognition_obligation_graphs WHERE episode_id=$1),
		       (SELECT COUNT(*) FROM cognition_transitions WHERE episode_id=$1),
		       (SELECT COUNT(*) FROM cognition_obligation_materialization_applications WHERE episode_id=$1)
		FROM cognition_actions actions JOIN cognition_episodes episodes USING (episode_id)
		WHERE actions.action_id=$2
	`, fixture.EpisodeID, action.Action.ID).Scan(
		&status, &revision, &graphVersion, &transitions, &applications,
	); err != nil {
		t.Fatal(err)
	}
	if status != CognitionActionDispatched || revision != 1 || graphVersion != 1 ||
		transitions != 1 || applications != 0 {
		t.Fatalf("rollback status=%q revision=%d graph=%d transitions=%d applications=%d",
			status, revision, graphVersion, transitions, applications)
	}
}

func reserveMaterializationChildTaskNode(t *testing.T, fixture cognitionDatabaseFixture) {
	t.Helper()
	desired, err := cognition.NewGoalExpression([]cognition.Predicate{{
		Name: "prerequisite", Args: []string{"artifact-1"},
	}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	childID, err := cognition.DeriveObligationID(
		fixture.EpisodeID, cognition.InitialObligationGeneration, fixture.Start.Root.ID,
		desired, fixture.Check,
	)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := taskstate.NewJSONObject([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	tx, err := fixture.Repository.pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	header, err := loadTaskLedgerHeaderTx(t.Context(), tx, fixture.Authority.JobID, true)
	if err != nil {
		t.Fatal(err)
	}
	commandID, err := cognitionTaskCommandID(string(fixture.EpisodeID), "reserve-materialization-child")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := applyQueueOwnedTaskCommandTx(
		t.Context(), tx, fixture.Authority.JobID, fixture.Authority.Generation,
		taskstate.AddNodeCommand{
			CommandID: commandID, ExpectedVersion: header.Version, Actor: taskstate.AuthorityCode,
			ID: taskstate.NodeID(childID), ParentID: taskstate.NodeID(fixture.Start.Root.ID),
			Kind: taskstate.NodeObjective, Title: "Reserved collision node", Priority: 10,
			CreatedStepID:      &fixture.Authority.StepID,
			AcceptanceCriteria: []string{"This test node intentionally reserves the exact identity."},
			Metadata:           metadata,
		},
	); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}
