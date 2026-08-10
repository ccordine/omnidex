package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
)

func TestPostgresCognitionGraphRejectsRemovedLegacyCommandKinds(t *testing.T) {
	_, repository, pool := openWorkingSetDatabase(t)
	fixture := newCognitionDatabaseFixture(t, repository)
	if _, err := repository.StartCognitionEpisode(t.Context(), fixture.Start, cognitionTestFactAuthority()); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"add", "activate", "add_dependency"} {
		_, err := pool.Exec(t.Context(), `
			INSERT INTO cognition_obligation_graphs (
				episode_id,graph_version,job_id,generation,step_id,command_id,
				command_sha256,command_kind,graph_json,graph_sha256,graph_json_sha256,
				actor_attempt,actor_worker_id
			)
			SELECT episode_id,graph_version+1,job_id,generation,step_id,
			       'cognition_graph_command_'||repeat($2,64),repeat($2,64),$1,
			       graph_json,graph_sha256,graph_json_sha256,actor_attempt,actor_worker_id
			FROM cognition_obligation_graphs WHERE episode_id=$3 AND graph_version=1
		`, kind, string(kind[0]), fixture.EpisodeID)
		if err == nil || !strings.Contains(err.Error(), "cognition_obligation_graphs_command_kind_check") {
			t.Fatalf("legacy graph kind %q error=%v, want command-kind constraint", kind, err)
		}
	}
}

func TestPostgresCognitionMaterializesOneChildOnlyAfterSuccessfulAction(t *testing.T) {
	_, repository, pool := openWorkingSetDatabase(t)
	fixture := newCognitionDatabaseFixture(t, repository)
	if _, err := repository.StartCognitionEpisode(t.Context(), fixture.Start, cognitionTestFactAuthority()); err != nil {
		t.Fatal(err)
	}
	action, _ := prepareCognitionProposalAction(t, fixture)
	before, err := repository.CognitionObligationGraph(t.Context(), fixture.EpisodeID)
	if err != nil {
		t.Fatal(err)
	}
	if before.Version != 1 || len(before.Graph.Obligations) != 1 ||
		before.Graph.Obligations[0].Status != cognition.ObligationActive {
		t.Fatalf("proposal mutated graph before action: %+v", before)
	}
	transition := cognitionProposalTransition(t, fixture, action)
	succeeded, err := repository.IngestCognitionTransition(
		t.Context(), fixture.Authority, action.Action.ID, transition, cognitionTestFactAuthority(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if succeeded.Status != CognitionActionSucceeded {
		t.Fatalf("succeeded action=%+v", succeeded)
	}
	after, err := repository.CognitionObligationGraph(t.Context(), fixture.EpisodeID)
	if err != nil {
		t.Fatal(err)
	}
	parent, child := materializedParentAndChild(t, after.Graph, fixture.Start.Root.ID)
	if after.Version != 2 || after.CommandKind != CognitionObligationMaterialize ||
		parent.Status != cognition.ObligationBlocked || child.Status != cognition.ObligationActive ||
		len(parent.DependsOn) != 1 || parent.DependsOn[0] != child.ID {
		t.Fatalf("materialized graph=%+v parent=%+v child=%+v", after, parent, child)
	}
	var parentTask, childTask taskstate.NodeStatus
	var descriptors, applications int
	if err := pool.QueryRow(t.Context(), `
		SELECT parent.status,child.status,
		       (SELECT COUNT(*) FROM cognition_obligation_materializations WHERE episode_id=$1),
		       (SELECT COUNT(*) FROM cognition_obligation_materialization_applications WHERE episode_id=$1)
		FROM task_nodes parent,task_nodes child
		WHERE parent.ledger_id=child.ledger_id AND parent.id=$2 AND child.id=$3
	`, fixture.EpisodeID, parent.ID, child.ID).Scan(
		&parentTask, &childTask, &descriptors, &applications,
	); err != nil {
		t.Fatal(err)
	}
	if parentTask != taskstate.NodeBlocked || childTask != taskstate.NodeActive ||
		descriptors != 1 || applications != 1 {
		t.Fatalf("task/materialization projection=%q/%q descriptors=%d applications=%d",
			parentTask, childTask, descriptors, applications)
	}
	replayed, err := repository.IngestCognitionTransition(
		t.Context(), fixture.Authority, action.Action.ID, transition, cognitionTestFactAuthority(),
	)
	if err != nil || replayed.Status != CognitionActionSucceeded {
		t.Fatalf("transition replay=%+v error=%v", replayed, err)
	}
	var graphs, applicationReplays int
	if err := pool.QueryRow(t.Context(), `
		SELECT (SELECT COUNT(*) FROM cognition_obligation_graphs WHERE episode_id=$1),
		       (SELECT COUNT(*) FROM cognition_obligation_materialization_applications WHERE episode_id=$1)
	`, fixture.EpisodeID).Scan(&graphs, &applicationReplays); err != nil {
		t.Fatal(err)
	}
	if graphs != 2 || applicationReplays != 1 {
		t.Fatalf("replay graph/application counts=%d/%d", graphs, applicationReplays)
	}
}

func TestPostgresFailedCognitionActionDoesNotMaterializeProposedObligation(t *testing.T) {
	_, repository, pool := openWorkingSetDatabase(t)
	fixture := newCognitionDatabaseFixture(t, repository)
	if _, err := repository.StartCognitionEpisode(t.Context(), fixture.Start, cognitionTestFactAuthority()); err != nil {
		t.Fatal(err)
	}
	action, _ := prepareCognitionProposalAction(t, fixture)
	failure, err := cognition.NewActionFailure(
		cognition.ActionFailurePreconditionFailed, action.Action, action.ExpectedRevision,
		"The public precondition is not satisfied.", []cognition.EvidenceRef{fixture.Evidence},
	)
	if err != nil {
		t.Fatal(err)
	}
	failed, err := repository.IngestCognitionFailure(
		t.Context(), fixture.Authority, action.Action.ID, failure,
	)
	if err != nil || failed.Status != CognitionActionFailed {
		t.Fatalf("failed action=%+v error=%v", failed, err)
	}
	graph, err := repository.CognitionObligationGraph(t.Context(), fixture.EpisodeID)
	if err != nil {
		t.Fatal(err)
	}
	if graph.Version != 1 || len(graph.Graph.Obligations) != 1 ||
		graph.Graph.Obligations[0].Status != cognition.ObligationActive {
		t.Fatalf("failed action materialized graph=%+v", graph)
	}
	var descriptors, applications, dispositions, rejected int
	if err := pool.QueryRow(t.Context(), `
		SELECT (SELECT COUNT(*) FROM cognition_obligation_materializations WHERE episode_id=$1),
		       (SELECT COUNT(*) FROM cognition_obligation_materialization_applications WHERE episode_id=$1),
		       (SELECT COUNT(*) FROM cognition_proposal_dispositions
		        WHERE episode_id=$1 AND outcome='rejected_action_failure'),
		       (SELECT COUNT(*) FROM task_entries WHERE ledger_id=(
		        SELECT ledger_id FROM cognition_episodes WHERE episode_id=$1)
		        AND metadata->>'source_kind'='model_obligation_candidate' AND status='rejected')
	`, fixture.EpisodeID).Scan(
		&descriptors, &applications, &dispositions, &rejected,
	); err != nil {
		t.Fatal(err)
	}
	if descriptors != 1 || applications != 0 || dispositions != 1 || rejected != 1 {
		t.Fatalf("failed descriptor/application/disposition/rejected=%d/%d/%d/%d",
			descriptors, applications, dispositions, rejected)
	}
}

func materializedParentAndChild(
	t *testing.T,
	graph cognition.ObligationGraphSnapshot,
	root cognition.ObligationID,
) (cognition.Obligation, cognition.Obligation) {
	t.Helper()
	var parent, child cognition.Obligation
	for _, obligation := range graph.Obligations {
		if obligation.ID == root {
			parent = obligation
		} else {
			child = obligation
		}
	}
	if parent.ID == "" || child.ID == "" || len(graph.Obligations) != 2 {
		t.Fatalf("materialized obligations=%+v", graph.Obligations)
	}
	return parent, child
}
