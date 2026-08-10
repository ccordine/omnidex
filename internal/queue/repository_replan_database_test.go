package queue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresReplanCreatesFreshCanonicalGeneration(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	marker := fmt.Sprintf("generation-replan-%d", time.Now().UnixNano())
	job, err := repository.EnqueueJob(ctx, marker, model.PipelineAssistant, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if job.CurrentGeneration != 1 {
		t.Fatalf("initial generation=%d", job.CurrentGeneration)
	}

	var planningID, analysisID int64
	if err := pool.QueryRow(ctx, `
		SELECT id FROM job_steps
		WHERE job_id=$1 AND action='v3_planning' AND generation=1
	`, job.ID).Scan(&planningID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT id FROM job_steps
		WHERE job_id=$1 AND action='v3_analysis' AND generation=1
	`, job.ID).Scan(&analysisID); err != nil {
		t.Fatal(err)
	}
	var delegatedID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO job_steps (job_id, action, sort_index, status, generation)
		VALUES ($1, 'v3_subtask', 45, $2, 1)
		RETURNING id
	`, job.ID, model.StepStatusCompleted).Scan(&delegatedID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE job_steps SET status=$3 WHERE job_id=$1 AND id=$2
	`, job.ID, planningID, model.StepStatusCompleted); err != nil {
		t.Fatal(err)
	}
	activateStepAttemptForTest(
		t, ctx, pool, job.ID, 1, analysisID,
		testStepAttemptWorker("replan-active", analysisID),
	)
	if _, err := pool.Exec(ctx, `
		UPDATE jobs SET status=$2, result='old', error='old', completed_at=NOW() WHERE id=$1
	`, job.ID, model.JobStatusRunning); err != nil {
		t.Fatal(err)
	}

	feedback := "Preserve accepted work and reconsider the remaining plan."
	replanned, err := repository.ReplanJob(ctx, testReplanCommand(t, job.ID, "canonical-generation", "  "+feedback+"  "))
	if err != nil {
		t.Fatal(err)
	}
	if replanned.ID != job.ID || replanned.CurrentGeneration != 2 || replanned.Status != model.JobStatusRunning ||
		replanned.Result != "" || replanned.Error != "" || replanned.CompletedAt != nil {
		t.Fatalf("replanned job=%+v", replanned)
	}
	digest := sha256.Sum256([]byte(feedback))
	var predecessor int64
	var purpose, boundary, storedFeedback, storedDigest string
	if err := pool.QueryRow(ctx, `
		SELECT predecessor_generation, purpose, boundary_action, feedback, feedback_sha256
		FROM job_generations WHERE job_id=$1 AND generation=2
	`, job.ID).Scan(&predecessor, &purpose, &boundary, &storedFeedback, &storedDigest); err != nil {
		t.Fatal(err)
	}
	if predecessor != 1 || purpose != jobGenerationPurposeReplan || boundary != replanPlanningBoundary ||
		storedFeedback != feedback || storedDigest != hex.EncodeToString(digest[:]) {
		t.Fatalf("generation record=%d/%s/%s/%q/%s", predecessor, purpose, boundary, storedFeedback, storedDigest)
	}
	ledger, err := repository.TaskLedger(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	feedbackID := replanFeedbackEntryID(2)
	entries := taskLedgerMutationEntries(ledger.Entries)
	entry, exists := entries[feedbackID]
	if !exists || entry.Kind != taskstate.EntryFeedback ||
		entry.FeedbackPurpose != taskstate.FeedbackReplan ||
		entry.Authority != taskstate.AuthorityUser || entry.Content != feedback ||
		entry.ScopeNodeID != initialTaskRootNodeID || len(entry.Refs) != 1 {
		t.Fatalf("replan feedback entry=%+v exists=%t", entry, exists)
	}
	wantRef := replanFeedbackRef(job.ID, 2, storedDigest)
	if entry.Refs[0] != wantRef {
		t.Fatalf("replan feedback ref=%+v want %+v", entry.Refs[0], wantRef)
	}
	var feedbackEventGeneration int64
	if err := pool.QueryRow(ctx, `
		SELECT job_generation FROM task_events
		WHERE job_id=$1 AND event_kind=$2 AND payload->'entry'->>'id'=$3
	`, job.ID, taskstate.EventEntryAdded, feedbackID).Scan(&feedbackEventGeneration); err != nil {
		t.Fatal(err)
	}
	if feedbackEventGeneration != 2 {
		t.Fatalf("feedback event generation=%d want 2", feedbackEventGeneration)
	}

	assertRetiredStep(t, ctx, pool, planningID, model.StepStatusCompleted, 2)
	assertRetiredStep(t, ctx, pool, delegatedID, model.StepStatusCompleted, 2)
	assertRetiredStep(t, ctx, pool, analysisID, model.StepStatusCanceled, 2)
	rows, err := pool.Query(ctx, `
		SELECT id, action, sort_index, status
		FROM job_steps
		WHERE job_id=$1 AND generation=2 AND superseded_at_generation IS NULL
		ORDER BY sort_index, id
	`, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	boundarySeeds, err := canonicalReplanTail(v3ConversationSteps())
	if err != nil {
		t.Fatal(err)
	}
	index := 0
	oldIDs := map[int64]bool{planningID: true, delegatedID: true, analysisID: true}
	for rows.Next() {
		var id int64
		var action, status string
		var sortIndex int
		if err := rows.Scan(&id, &action, &sortIndex, &status); err != nil {
			t.Fatal(err)
		}
		if index >= len(boundarySeeds.seeds) {
			t.Fatalf("unexpected generation-2 step %s@%d", action, sortIndex)
		}
		want := boundarySeeds.seeds[index]
		if oldIDs[id] || action != want.action || sortIndex != want.sortIndex || status != model.StepStatusPending {
			t.Fatalf("generation-2 step=%d/%s/%d/%s want fresh %s/%d/pending", id, action, sortIndex, status, want.action, want.sortIndex)
		}
		index++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if index != len(boundarySeeds.seeds) {
		t.Fatalf("generation-2 canonical step count=%d want %d", index, len(boundarySeeds.seeds))
	}
	var delegatedClones, feedbackContexts int
	if err := pool.QueryRow(ctx, `
		SELECT
		  COUNT(*) FILTER (WHERE generation=2 AND action='v3_subtask'),
		  (SELECT COUNT(*) FROM step_contexts c JOIN job_steps s ON s.id=c.step_id
		   WHERE s.job_id=$1 AND c.key IN ('replan_feedback', 'user_feedback'))
		FROM job_steps WHERE job_id=$1
	`, job.ID).Scan(&delegatedClones, &feedbackContexts); err != nil {
		t.Fatal(err)
	}
	if delegatedClones != 0 || feedbackContexts != 0 {
		t.Fatalf("delegated clones=%d feedback contexts=%d", delegatedClones, feedbackContexts)
	}
}

func TestPostgresReplanRejectsAssignedRetiringStepAtomically(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	marker := fmt.Sprintf("generation-assignment-%d", time.Now().UnixNano())
	job, err := repository.EnqueueJob(ctx, marker, model.PipelineCoding, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	var stepID int64
	if err := pool.QueryRow(ctx, `SELECT id FROM job_steps WHERE job_id=$1`, job.ID).Scan(&stepID); err != nil {
		t.Fatal(err)
	}
	addID, _ := taskstate.NewCommandID(marker, "add-task")
	if _, err := repository.ApplyTaskCommand(ctx, job.ID, 1, taskstate.AddNodeCommand{
		CommandID: addID, ExpectedVersion: initialTaskLedgerVersion, Actor: taskstate.AuthorityCode,
		ID: "assigned-task", Kind: taskstate.NodeTask, Title: "Assigned task", Priority: 50,
		AcceptanceCriteria: []string{}, Metadata: taskstate.EmptyJSONObject(),
	}); err != nil {
		t.Fatal(err)
	}
	assignID, _ := taskstate.NewCommandID(marker, "assign-task")
	if _, err := repository.ApplyTaskCommand(ctx, job.ID, 1, taskstate.AssignNodeStepCommand{
		CommandID: assignID, ExpectedVersion: initialTaskLedgerVersion + 1, Actor: taskstate.AuthorityCode,
		NodeID: "assigned-task", StepID: stepID,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET status=$2 WHERE id=$1`, job.ID, model.JobStatusRunning); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ReplanJob(ctx, testReplanCommand(t, job.ID, "assigned-tail", "Reconsider the change surface.")); !errors.Is(err, ErrInvalidJobGeneration) || !strings.Contains(err.Error(), "assigned-task") {
		t.Fatalf("assigned step replan error=%v", err)
	}
	var generation, generations int64
	var superseded *int64
	var status string
	if err := pool.QueryRow(ctx, `
		SELECT jobs.current_generation,
		       (SELECT COUNT(*) FROM job_generations g WHERE g.job_id=jobs.id),
		       steps.superseded_at_generation, steps.status
		FROM jobs JOIN job_steps steps ON steps.job_id=jobs.id AND steps.id=$2
		WHERE jobs.id=$1
	`, job.ID, stepID).Scan(&generation, &generations, &superseded, &status); err != nil {
		t.Fatal(err)
	}
	if generation != 1 || generations != 1 || superseded != nil || status != model.StepStatusPending {
		t.Fatalf("rejected replan mutated authority: gen=%d rows=%d superseded=%v status=%s", generation, generations, superseded, status)
	}
}

func replanTestRepository(t *testing.T) (*Repository, *pgxpool.Pool, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	t.Cleanup(cancel)
	pool := openIsolatedMigrationPool(t)
	t.Setenv("MIGRATIONS_DIR", filepath.Join("..", "..", "migrations"))
	repository := New(pool)
	if err := repository.EnsureSchema(ctx); err != nil {
		t.Fatal(err)
	}
	return repository, pool, ctx
}

func cancelOpenTaskLedgerTestJobs(
	t *testing.T,
	repository *Repository,
	pool *pgxpool.Pool,
	ctx context.Context,
) {
	t.Helper()
	rows, err := pool.Query(ctx, `
		SELECT jobs.id
		FROM jobs
		JOIN task_ledgers ON task_ledgers.job_id=jobs.id
		WHERE jobs.status IN ($1, $2, $3)
		ORDER BY jobs.id
	`, model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting)
	if err != nil {
		t.Fatal(err)
	}
	jobIDs := make([]int64, 0)
	for rows.Next() {
		var jobID int64
		if err := rows.Scan(&jobID); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		jobIDs = append(jobIDs, jobID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatal(err)
	}
	rows.Close()
	for _, jobID := range jobIDs {
		if _, err := repository.CancelJob(ctx, testCancelCommand(
			t, jobID, "replan-test-isolation", "isolate PostgreSQL queue test authority",
		)); err != nil {
			t.Fatalf("cancel prior test job %d: %v", jobID, err)
		}
	}
}

func assertRetiredStep(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	stepID int64,
	wantStatus string,
	wantSuperseded int64,
) {
	t.Helper()
	var status string
	var superseded int64
	if err := pool.QueryRow(ctx, `
		SELECT status, superseded_at_generation FROM job_steps WHERE id=$1
	`, stepID).Scan(&status, &superseded); err != nil {
		t.Fatal(err)
	}
	if status != wantStatus || superseded != wantSuperseded {
		t.Fatalf("retired step %d=%s/%d want %s/%d", stepID, status, superseded, wantStatus, wantSuperseded)
	}
}
