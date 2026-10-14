package queue

import (
	"bytes"
	"context"
	"errors"
	"os"
	"slices"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func freshAgentCodingClaim(t *testing.T) (*pgxpool.Pool, *Repository, model.Job, *model.ClaimedStep, model.CodingPlanLeafID) {
	t.Helper()
	pool, repository := freshCodingPlanRepository(t)
	repository.WithCodingAgents(codingAgentTestConfiguration())
	ctx := context.Background()
	job, err := repository.EnqueueCodingJobMode(ctx, "Implement approved behavior.", t.TempDir(), codingagent.Agents)
	if err != nil {
		t.Fatal(err)
	}
	planner, err := repository.ClaimNextStep(ctx, "agent-plan-fixture")
	if err != nil || planner == nil {
		t.Fatalf("claim planning: %v, %v", planner, err)
	}
	command, leaf := codingAgentPlanReviewFixture(t, repository, planner, "Approved behavior works")
	plan := freezeCodingAgentPlanFixture(t, repository, job, command)
	if plan.State != model.CodingPlanStateFrozen {
		t.Fatalf("Agents planning was not explicitly frozen: %#v", plan)
	}
	claim, err := repository.ClaimNextStep(ctx, "agent-execution-fixture")
	if err != nil || claim == nil || claim.Step.Action != "v3_coding" {
		t.Fatalf("claim execution: %v, %v", claim, err)
	}
	return pool, repository, job, claim, leaf.Leaf.ID
}

func TestCodingAgentQuestionsResumeSameGenerationAndReplayHistorically(t *testing.T) {
	pool, repository, job, claim, _ := freshAgentCodingClaim(t)
	ctx := context.Background()
	store := repository.CodingAgentEvidence(claim.Authority)
	wire := []byte(`{  "z": "\u0061", "a": [ 1, 2 ] }` + "\n")
	wireID, err := store.Append(ctx, codingagent.Entry{Generation: claim.Authority.Generation, Phase: "native_event", Frame: wire})
	if err != nil {
		t.Fatal(err)
	}
	var recordedWire []byte
	if err := pool.QueryRow(ctx, `SELECT decode(payload_json->>'frame','base64') FROM evidence WHERE id=$1`, wireID).Scan(&recordedWire); err != nil || !bytes.Equal(recordedWire, wire) {
		t.Fatalf("native receipt changed original wire bytes: %q %v", recordedWire, err)
	}
	questions := []string{"Which approved interaction should be used?"}
	questionID, err := store.Append(ctx, codingagent.Entry{Generation: claim.Authority.Generation, Phase: "questions", Questions: questions})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.PauseCodingAgentStep(ctx, claim.Authority, []string{"Different question"}); err == nil {
		t.Fatal("pause accepted a question without exact durable evidence")
	}
	if err := repository.PauseCodingAgentStep(ctx, claim.Authority, questions); err != nil {
		t.Fatal(err)
	}
	pause, err := repository.ReadCodingAgentPause(ctx, job.ID, claim.Authority.Generation)
	if err != nil || pause == nil || pause.Schema != "omnidex-coding-agent-pause-v1" || pause.Generation != claim.Authority.Generation || pause.StepID != claim.Authority.StepID || pause.WaitingAttempt != claim.Authority.Attempt || !slices.Equal(pause.Questions, questions) {
		t.Fatalf("durable pause projection lost exact context: %#v %v", pause, err)
	}
	if next, err := repository.ClaimNextStep(ctx, "premature-agent-fixture"); err != nil || next != nil {
		t.Fatalf("unanswered step was claimed: %v, %v", next, err)
	}
	root, identity := codingPlanWorkspaceAuthority(t, job)
	command := SubmitJobFeedbackCommand{OperationID: codingPlanOperationID(t, "answer", job.ID), ExpectedGeneration: &claim.Authority.Generation, ExpectedWaitingAttempt: &claim.Authority.Attempt, JobID: job.ID, Feedback: "Use the approved interaction.", WorkspaceRoot: root, WorkspaceIdentity: identity}
	result, err := repository.SubmitJobFeedback(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Applied || result.Job.Status != model.JobStatusRunning || result.Job.CurrentGeneration != 1 {
		t.Fatalf("answer did not resume same generation: %#v", result)
	}
	if pause, err := repository.ReadCodingAgentPause(ctx, job.ID, claim.Authority.Generation); pause != nil || !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("answered question still projects as paused: %#v %v", pause, err)
	}
	maintenance, err := os.ReadFile("../../database/agent_execution_maintenance.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(maintenance)); err != nil {
		t.Fatalf("explicit maintenance rejected durable owner state: %v", err)
	}
	var persistedQuestion int64
	if err := pool.QueryRow(ctx, `SELECT (payload_json->>'question_id')::bigint FROM evidence WHERE job_id=$1 AND kind='coding_agent' AND payload_json->>'operation_id'=$2`, job.ID, command.OperationID).Scan(&persistedQuestion); err != nil || persistedQuestion != questionID {
		t.Fatalf("answer lost question binding: %d %v", persistedQuestion, err)
	}
	resumed, err := repository.ClaimNextStep(ctx, "resumed-agent-fixture")
	if err != nil || resumed == nil || resumed.Authority.StepID != claim.Authority.StepID || resumed.Authority.Generation != 1 || resumed.Authority.Attempt != claim.Authority.Attempt+1 {
		t.Fatalf("answer was not immediately reclaimed: %v, %v", resumed, err)
	}
	if _, err := store.Append(ctx, codingagent.Entry{Generation: 1, Phase: "overall_accepted"}); !errors.Is(err, ErrStaleStepAttempt) {
		t.Fatalf("previous attempt retained write authority: %v", err)
	}
	laterQuestions := []string{"Choose one remaining approved detail."}
	if _, err := repository.CodingAgentEvidence(resumed.Authority).Append(ctx, codingagent.Entry{Generation: 1, Phase: "questions", Questions: laterQuestions}); err != nil {
		t.Fatal(err)
	}
	if err := repository.PauseCodingAgentStep(ctx, resumed.Authority, laterQuestions); err != nil {
		t.Fatal(err)
	}
	stale := command
	stale.OperationID = codingPlanOperationID(t, "stale-waiting-attempt", job.ID)
	if _, err := repository.SubmitJobFeedback(ctx, stale); !errors.Is(err, ErrJobFeedbackWaitingAttempt) {
		t.Fatalf("first pause reply answered a later same-generation question: %v", err)
	}
	replay, err := repository.SubmitJobFeedback(ctx, command)
	if err != nil || replay.Applied || replay.Job.Status != model.JobStatusRunning {
		t.Fatalf("historical answer replay failed after later question: %#v, %v", replay, err)
	}
	command.Feedback = "Changed answer."
	if _, err := repository.SubmitJobFeedback(ctx, command); !errors.Is(err, ErrLifecycleOperationConflict) {
		t.Fatalf("same operation accepted changed answer: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE evidence SET payload_json='{}'::jsonb WHERE id=$1`, questionID); err == nil {
		t.Fatal("agent receipt was editable")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM evidence WHERE id=$1`, questionID); err == nil {
		t.Fatal("agent receipt was deletable")
	}
}

func TestCodingAgentCompletionTransactionRequiresCurrentAcceptance(t *testing.T) {
	_, repository, job, claim, _ := freshAgentCodingClaim(t)
	ctx := context.Background()
	operation, err := NewStepLifecycleOperationID(claim.Authority, LifecycleCompleteStep)
	if err != nil {
		t.Fatal(err)
	}
	command := CompleteStepCommand{OperationID: operation, Authority: claim.Authority, StepID: claim.Step.ID, Output: "Completed approved behavior."}
	if err := repository.CompleteStep(ctx, command); err == nil {
		t.Fatal("job completed without accepted agent evidence")
	}
	approved, err := repository.CodingAgentFrozenWorkload(ctx, claim.Authority)
	if err != nil {
		t.Fatal(err)
	}
	fixture := codingAgentCompletionJournalFixture()
	mapping := map[int64]int64{}
	store := repository.CodingAgentEvidence(claim.Authority)
	for _, entry := range fixture {
		if entry.Role == codingagent.Plan || entry.Phase == "plan_frozen" {
			continue
		}
		oldID := entry.ID
		entry.ID = 0
		if entry.CallID != 0 {
			entry.CallID = mapping[entry.CallID]
		}
		if entry.Result != nil {
			entry.Result.CallID = entry.CallID
		}
		if entry.Assignment != nil {
			if entry.Assignment.Role == codingagent.Review {
				entry.Assignment.Task = &approved.Tasks[0]
				entry.Assignment.ReadFiles = append(append([]string{}, approved.Tasks[0].ReadFiles...), approved.Tasks[0].WriteFiles...)
			}
			if entry.Assignment.Role == codingagent.OverallReview {
				entry.Assignment.Requirements = []codingagent.Requirement{{ID: "objective", Outcome: job.Instruction}}
				entry.Assignment.Objective = &codingagent.Objective{Instruction: job.Instruction}
				entry.Assignment.Workload = approved
			}
			entry.Assignment.JobID = job.ID
			entry.Assignment.Generation = 1
		}
		if entry.TaskID != "" {
			entry.TaskID = approved.Tasks[0].ID
		}
		if entry.Phase == "task_accepted" {
			entry.Checks = []codingagent.CheckResult{{ID: approved.Tasks[0].Checks[0].ID}}
		}
		if entry.Phase == "overall_accepted" {
			entry.Checks = []codingagent.CheckResult{{ID: approved.Checks[0].ID}}
		}
		id, err := store.Append(ctx, entry)
		if err != nil {
			t.Fatal(err)
		}
		mapping[oldID] = id
	}
	if err := repository.CompleteStep(ctx, command); err != nil {
		t.Fatal(err)
	}
	if err := repository.CompleteStep(ctx, command); err != nil {
		t.Fatalf("completion replay failed: %v", err)
	}
	details, err := repository.CurrentJobDetails(ctx, job.ID)
	if err != nil || details.Job.Status != model.JobStatusCompleted {
		t.Fatalf("accepted job did not complete: %#v, %v", details.Job, err)
	}
}

func TestCodingAgentWaitingCancellationRevokesResumeAuthority(t *testing.T) {
	_, repository, job, claim, _ := freshAgentCodingClaim(t)
	ctx := context.Background()
	questions := []string{"Choose an approved option."}
	store := repository.CodingAgentEvidence(claim.Authority)
	if _, err := store.Append(ctx, codingagent.Entry{Generation: 1, Phase: "questions", Questions: questions}); err != nil {
		t.Fatal(err)
	}
	if err := repository.PauseCodingAgentStep(ctx, claim.Authority, questions); err != nil {
		t.Fatal(err)
	}
	root, identity := codingPlanWorkspaceAuthority(t, job)
	if _, err := repository.CancelJob(ctx, CancelJobCommand{OperationID: codingPlanOperationID(t, "cancel", job.ID), JobID: job.ID, Reason: "Stop approved work.", WorkspaceRoot: root, WorkspaceIdentity: identity}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Append(ctx, codingagent.Entry{Generation: 1, Phase: "overall_accepted"}); !errors.Is(err, ErrStaleStepAttempt) {
		t.Fatalf("canceled attempt retained evidence authority: %v", err)
	}
	if next, err := repository.ClaimNextStep(ctx, "canceled-agent-fixture"); err != nil || next != nil {
		t.Fatalf("canceled question reclaimed: %v, %v", next, err)
	}
	if _, err := repository.SubmitJobFeedback(ctx, SubmitJobFeedbackCommand{OperationID: codingPlanOperationID(t, "late-answer", job.ID), ExpectedGeneration: &claim.Authority.Generation, ExpectedWaitingAttempt: &claim.Authority.Attempt, JobID: job.ID, Feedback: "A late answer.", WorkspaceRoot: root, WorkspaceIdentity: identity}); !errors.Is(err, ErrStepNotWritable) {
		t.Fatalf("canceled job resumed: %v", err)
	}
}
