package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresAcceptedIntentArtifactProjectsOneBoundedAuthority(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	marker := fmt.Sprintf("accepted-intent-%d", time.Now().UnixNano())
	job, err := repository.EnqueueJob(ctx, marker, model.PipelineAssistant, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := repository.ClaimNextStep(ctx, marker+"-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claimed == nil || claimed.Job.ID != job.ID || claimed.Step.Action != "v3_intent_parse" {
		t.Fatalf("claimed step=%+v, want job %d intent parse", claimed, job.ID)
	}
	envelope := acceptedIntentTestEnvelope(t, job.ID, claimed.Step.ID)
	if err := repository.WriteAcceptedIntentArtifact(ctx, envelope); err != nil {
		t.Fatal(err)
	}
	if err := repository.WriteAcceptedIntentArtifact(ctx, envelope); err != nil {
		t.Fatalf("exact accepted-intent replay failed: %v", err)
	}

	var artifactsCount, projections, items, version int64
	if err := pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM artifacts WHERE job_id=$1 AND kind='intent'),
			(SELECT COUNT(*) FROM task_artifact_projections WHERE job_id=$1),
			(SELECT COUNT(*) FROM task_artifact_projection_items WHERE job_id=$1),
			(SELECT version FROM task_ledgers WHERE job_id=$1)
	`, job.ID).Scan(&artifactsCount, &projections, &items, &version); err != nil {
		t.Fatal(err)
	}
	if artifactsCount != 1 || projections != 1 || items != 3 || version != 10 {
		t.Fatalf("artifact/projection/items/version=%d/%d/%d/%d", artifactsCount, projections, items, version)
	}
	state, err := repository.TaskLedger(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	objective := acceptedIntentObjective(t, state)
	if objective.ID == "goal:root" || objective.Status != taskstate.NodeActive ||
		objective.AssignedStepID != nil || objective.CreatedStepID == nil ||
		*objective.CreatedStepID != claimed.Step.ID {
		t.Fatalf("projected objective=%+v", objective)
	}
	constraints, questions := 0, 0
	for _, entry := range state.Entries {
		switch {
		case entry.Kind == taskstate.EntryConstraint && entry.ScopeNodeID == objective.ID:
			constraints++
			if entry.Authority != taskstate.AuthorityCode || len(entry.Refs) != 1 {
				t.Fatalf("projected constraint=%+v", entry)
			}
		case entry.Kind == taskstate.EntryQuestion && entry.ScopeNodeID == objective.ID:
			questions++
			if entry.Authority != taskstate.AuthorityModelProposal || len(entry.Refs) != 1 {
				t.Fatalf("projected ambiguity=%+v", entry)
			}
		}
	}
	if constraints != 1 || questions != 1 {
		t.Fatalf("projected constraints/questions=%d/%d", constraints, questions)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE artifacts SET payload_json='{}'::jsonb
		WHERE job_id=$1 AND kind='intent'
	`, job.ID); err == nil || !strings.Contains(err.Error(), "accepted task artifact is immutable") {
		t.Fatalf("projected artifact mutation error=%v", err)
	}
	if _, err := pool.Exec(ctx, `
		DELETE FROM task_artifact_projection_items WHERE job_id=$1
	`, job.ID); err == nil || !strings.Contains(err.Error(), "task artifact projections are immutable") {
		t.Fatalf("projection item deletion error=%v", err)
	}
	unbound, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := unbound.Exec(ctx, `
		INSERT INTO artifacts (job_id, step_id, kind, version, payload_json)
		VALUES ($1, $2, 'intent', '1', '{}'::jsonb)
	`, job.ID, claimed.Step.ID); err != nil {
		unbound.Rollback(ctx)
		t.Fatal(err)
	}
	if err := unbound.Commit(ctx); err == nil ||
		!strings.Contains(err.Error(), "intent artifact requires an accepted task projection") {
		t.Fatalf("unbound intent artifact commit error=%v", err)
	}
	commandID, err := taskstate.NewCommandID(marker, "forbidden-objective-mutation")
	if err != nil {
		t.Fatal(err)
	}
	_, err = repository.ApplyTaskCommand(ctx, job.ID, 1, taskstate.TransitionNodeCommand{
		CommandID: commandID, ExpectedVersion: uint64(version), Actor: taskstate.AuthorityCode,
		NodeID: objective.ID, To: taskstate.NodeFailed, Reason: "External mutation is forbidden.",
	})
	if !errors.Is(err, taskstate.ErrAuthorityDenied) {
		t.Fatalf("public projected-objective mutation error=%v", err)
	}

	changed := acceptedIntentTestEnvelope(t, job.ID, claimed.Step.ID)
	var intent artifacts.IntentArtifact
	if err := json.Unmarshal(changed.Payload, &intent); err != nil {
		t.Fatal(err)
	}
	intent.Constraints[0] = "Change the accepted constraint"
	changed, err = artifacts.MarshalPayload(artifacts.KindIntent, "1", intent)
	if err != nil {
		t.Fatal(err)
	}
	changed.JobID, changed.StepID = job.ID, claimed.Step.ID
	if err := repository.WriteAcceptedIntentArtifact(ctx, changed); err == nil {
		t.Fatal("changed accepted intent replay was accepted")
	}
}

func TestPostgresAcceptedIntentProjectionFailureRollsBackArtifactAndLedger(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	marker := fmt.Sprintf("accepted-intent-rollback-%d", time.Now().UnixNano())
	job, err := repository.EnqueueJob(ctx, marker, model.PipelineAssistant, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := repository.ClaimNextStep(ctx, marker+"-worker")
	if err != nil {
		t.Fatal(err)
	}
	installAcceptedIntentProjectionFailure(t, ctx, pool, job.ID)
	if err := repository.WriteAcceptedIntentArtifact(
		ctx, acceptedIntentTestEnvelope(t, job.ID, claimed.Step.ID),
	); err == nil || !strings.Contains(err.Error(), "forced accepted intent projection failure") {
		t.Fatalf("forced projection failure error=%v", err)
	}
	var artifactsCount, projectionCount, nodes, entries, version int64
	if err := pool.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM artifacts WHERE job_id=$1 AND kind='intent'),
			(SELECT COUNT(*) FROM task_artifact_projections WHERE job_id=$1),
			(SELECT COUNT(*) FROM task_nodes WHERE job_id=$1),
			(SELECT COUNT(*) FROM task_entries WHERE job_id=$1),
			(SELECT version FROM task_ledgers WHERE job_id=$1)
	`, job.ID).Scan(&artifactsCount, &projectionCount, &nodes, &entries, &version); err != nil {
		t.Fatal(err)
	}
	if artifactsCount != 0 || projectionCount != 0 || nodes != 1 || entries != 1 || version != 4 {
		t.Fatalf(
			"rollback artifact/projection/nodes/entries/version=%d/%d/%d/%d/%d",
			artifactsCount, projectionCount, nodes, entries, version,
		)
	}
}

func acceptedIntentTestEnvelope(t *testing.T, jobID, stepID int64) artifacts.Envelope {
	t.Helper()
	intent := artifacts.IntentArtifact{
		UserGoal: "Implement the requested change", Mode: "execute",
		RequiresAction: true, MemoryMode: artifacts.MemoryModeOff,
		Objectives: []artifacts.Objective{{
			ID: "goal:root", Description: "Implement the accepted change", Priority: 90,
			RequiresAction: true, RequiredCapabilities: []string{},
			AcceptanceCriteria: []string{"Focused proof passes"},
		}},
		Constraints:          []string{"Preserve existing behavior"},
		RequiredCapabilities: []string{}, CompletionCriteria: []string{"Job proof passes"},
		UnresolvedReferences: []string{},
		Ambiguities:          []string{"Does the existing caller require compatibility?"},
	}
	envelope, err := artifacts.MarshalPayload(artifacts.KindIntent, "1", intent)
	if err != nil {
		t.Fatal(err)
	}
	envelope.JobID, envelope.StepID = jobID, stepID
	return envelope
}

func acceptedIntentObjective(t *testing.T, state taskstate.MaterializedState) taskstate.Node {
	t.Helper()
	for _, node := range state.Nodes {
		if node.Kind == taskstate.NodeObjective {
			return node
		}
	}
	t.Fatal("accepted intent objective is missing")
	return taskstate.Node{}
}

func installAcceptedIntentProjectionFailure(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	jobID int64,
) {
	t.Helper()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	functionName := "accepted_intent_projection_failure_" + suffix
	triggerName := "accepted_intent_projection_failure_trigger_" + suffix
	if _, err := pool.Exec(ctx, fmt.Sprintf(`
		CREATE FUNCTION %s() RETURNS trigger AS $$
		BEGIN
			IF NEW.job_id=%d THEN
				RAISE EXCEPTION 'forced accepted intent projection failure';
			END IF;
			RETURN NEW;
		END;
		$$ LANGUAGE plpgsql;
		CREATE TRIGGER %s BEFORE INSERT ON task_artifact_projections
		FOR EACH ROW EXECUTE FUNCTION %s()
	`, functionName, jobID, triggerName, functionName)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = pool.Exec(cleanupCtx, fmt.Sprintf(
			"DROP TRIGGER %s ON task_artifact_projections; DROP FUNCTION %s()",
			triggerName, functionName,
		))
	})
}
