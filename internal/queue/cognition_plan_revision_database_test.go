package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
)

func TestPostgresCognitionPlanRevisionCutoverIsAtomicAndReplayable(t *testing.T) {
	_, repository, pool := openWorkingSetDatabase(t)
	fixture := newCognitionDatabaseFixture(t, repository)
	if _, err := repository.StartCognitionEpisode(
		t.Context(), fixture.Start, cognitionTestFactAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	action, _ := prepareCognitionPlanRevisionAction(t, fixture)
	before, err := repository.CognitionObligationGraph(t.Context(), fixture.EpisodeID)
	if err != nil || before.Graph.Generation != cognition.InitialObligationGeneration {
		t.Fatalf("initial graph=%+v error=%v", before, err)
	}
	transition := cognitionProposalTransition(t, fixture, action)
	if _, err := repository.IngestCognitionTransition(
		t.Context(), fixture.Authority, action.Action.ID, transition, cognitionTestFactAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	after, err := repository.CognitionObligationGraph(t.Context(), fixture.EpisodeID)
	if err != nil {
		t.Fatal(err)
	}
	old, root, next := planRevisionObligations(t, after.Graph)
	if after.Version != 2 || after.CommandKind != CognitionObligationPlanRevise ||
		after.Graph.Generation != 2 || old.Status != cognition.ObligationSuperseded ||
		root.Status != cognition.ObligationBlocked || next.Status != cognition.ObligationActive ||
		len(root.DependsOn) != 1 || root.DependsOn[0] != next.ID {
		t.Fatalf("revised graph=%+v old=%+v root=%+v next=%+v", after, old, root, next)
	}
	prepared, err := repository.PrepareCognitionRuntimeSnapshot(
		t.Context(), CognitionRuntimeSnapshotCommand{
			Authority: fixture.Authority, EpisodeID: fixture.EpisodeID,
		},
	)
	if err != nil {
		t.Fatalf("prepare generation-two plan under original worker generation: %v", err)
	}
	if prepared.Prepared.ObligationGraph.Generation != 2 ||
		prepared.Prepared.Snapshot.Attempt().Generation != fixture.Authority.Generation ||
		prepared.Prepared.Snapshot.CurrentObligation().ID != next.ID {
		t.Fatalf("prepared independent generations=%+v", prepared.Prepared)
	}
	var oldTask, rootTask, nextTask taskstate.NodeStatus
	var revisions, applications, dispositions, sources, supersessions int
	if err := pool.QueryRow(t.Context(), `
		SELECT old.status,root.status,next.status,
		       (SELECT COUNT(*) FROM cognition_plan_revisions WHERE episode_id=$1),
		       (SELECT COUNT(*) FROM cognition_plan_revision_applications WHERE episode_id=$1),
		       (SELECT COUNT(*) FROM cognition_proposal_dispositions WHERE episode_id=$1),
		       (SELECT COUNT(*) FROM cognition_graph_materialization_sources WHERE episode_id=$1),
		       (SELECT COUNT(*) FROM task_node_generation_supersessions
		        WHERE ledger_id=old.ledger_id AND node_id=old.id)
		FROM task_nodes old,task_nodes root,task_nodes next
		WHERE old.ledger_id=root.ledger_id AND root.ledger_id=next.ledger_id
		  AND old.id=$2 AND root.id=$3 AND next.id=$4
	`, fixture.EpisodeID, old.ID, root.ID, next.ID).Scan(
		&oldTask, &rootTask, &nextTask, &revisions, &applications,
		&dispositions, &sources, &supersessions,
	); err != nil {
		t.Fatal(err)
	}
	if oldTask != taskstate.NodeCanceled || rootTask != taskstate.NodeBlocked ||
		nextTask != taskstate.NodeActive || revisions != 1 || applications != 1 ||
		dispositions != 1 || sources != 1 || supersessions != 1 {
		t.Fatalf("task=%q/%q/%q rows=%d/%d/%d/%d supersessions=%d",
			oldTask, rootTask, nextTask, revisions, applications, dispositions, sources, supersessions)
	}
	if _, err := repository.IngestCognitionTransition(
		t.Context(), fixture.Authority, action.Action.ID, transition, cognitionTestFactAuthority(),
	); err != nil {
		t.Fatalf("exact transition replay: %v", err)
	}
}

func TestPostgresCognitionPlanRevisionTraceRejectsForgeryAndMutation(t *testing.T) {
	_, repository, pool := openWorkingSetDatabase(t)
	fixture := newCognitionDatabaseFixture(t, repository)
	if _, err := repository.StartCognitionEpisode(
		t.Context(), fixture.Start, cognitionTestFactAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	action, _ := prepareCognitionPlanRevisionAction(t, fixture)
	if _, err := repository.IngestCognitionTransition(
		t.Context(), fixture.Authority, action.Action.ID,
		cognitionProposalTransition(t, fixture, action), cognitionTestFactAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	var digest string
	if err := pool.QueryRow(t.Context(), `
		SELECT descriptor_json,descriptor_json_sha256
		FROM cognition_plan_revisions WHERE episode_id=$1
	`, fixture.EpisodeID).Scan(&raw, &digest); err != nil {
		t.Fatal(err)
	}
	if _, err := validateCognitionPlanRevisionTracePayload(raw, digest); err != nil {
		t.Fatalf("valid plan revision trace: %v", err)
	}
	var forged map[string]any
	if err := json.Unmarshal(raw, &forged); err != nil {
		t.Fatal(err)
	}
	forged["result_graph_sha256"] = cognitionTestDigest("8")
	changed, err := json.Marshal(forged)
	if err != nil {
		t.Fatal(err)
	}
	changedDigest := sha256.Sum256(changed)
	if _, err := validateCognitionPlanRevisionTracePayload(
		changed, hex.EncodeToString(changedDigest[:]),
	); err == nil {
		t.Fatal("self-consistently rehashed forged plan revision entered the sealed trace")
	}
	if _, err := pool.Exec(t.Context(), `
		UPDATE cognition_plan_revisions SET result_graph_sha256=$2 WHERE episode_id=$1
	`, fixture.EpisodeID, cognitionTestDigest("8")); err == nil {
		t.Fatal("durable plan revision authority was mutable")
	}
}
