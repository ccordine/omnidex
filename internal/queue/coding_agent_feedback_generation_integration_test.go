package queue

import (
	"context"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestAgentsPlanReviewCannotBecomeFeedback(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJobMode(ctx, "Review the finite plan.", t.TempDir(), codingagent.Agents)
	if err != nil {
		t.Fatal(err)
	}
	planner, err := repository.ClaimNextStep(ctx, "no-agent-feedback-planner")
	if err != nil || planner == nil || planner.Step.Action != "v3_coding_plan" {
		t.Fatalf("claim planning step = %#v, %v", planner, err)
	}
	fixture := codingAgentPlanReviewFixture(t, repository, planner, "Review the finite plan.")
	if err := repository.MarkCodingLedgerReviewReady(ctx, planner.Authority, fixture.created.LedgerID, fixture.created.Revision); err != nil {
		t.Fatal(err)
	}
	root, identity := codingPlanWorkspaceAuthority(t, job)
	operationID := codingPlanOperationID(t, "forbidden-agent-feedback", job.ID)
	_, err = repository.SubmitJobFeedback(ctx, SubmitJobFeedbackCommand{
		OperationID: operationID, JobID: job.ID, ExpectedGeneration: &job.CurrentGeneration,
		Feedback: "Treat this as an answer.", WorkspaceRoot: root, WorkspaceIdentity: identity,
	})
	if !errors.Is(err, ErrStepNotWritable) {
		t.Fatalf("Agents plan review feedback error = %v", err)
	}
	var operationCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM job_lifecycle_operations WHERE operation_id=$1`, operationID).Scan(&operationCount); err != nil {
		t.Fatal(err)
	}
	if operationCount != 0 {
		t.Fatalf("rejected Agents feedback persisted %d lifecycle operations", operationCount)
	}
}

func TestPlanningReplanRejectsStaleLedgerRevisionBeforeGenerationAdvance(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "Amend the displayed proposal.", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ledger := repositoryLedgerFixture()
	ids := make([]model.LedgerItemID, 0)
	for _, record := range ledgerRepositoryRecords(ledger) {
		setCodingLedgerItem(&ledger, record.item.ID, func(item *model.LedgerItem) { item.ReviewState = model.LedgerReviewProposed })
		ids = append(ids, record.item.ID)
	}
	created, err := repository.CreateCodingLedger(ctx, CreateCodingLedgerCommand{
		JobID: job.ID, OperationID: "stale-replan-create", OwnerTurnID: "stale-replan-create-turn",
		OwnerTurnText: "Create the proposal.", Ledger: ledger, Routes: repositoryLedgerRoutes(1),
		OwnerTurnDisposition: model.RepresentedOwnerTurn(ids),
	})
	if err != nil {
		t.Fatal(err)
	}
	current, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: created.LedgerID, OwnerTurnText: "Agree the task.",
		Mutation: model.LedgerMutation{OperationID: "stale-replan-change", BaseRevision: created.Revision, OwnerTurnID: "stale-replan-change-turn",
			OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{"task"}),
			Operations:           []model.LedgerMutationOperation{{Kind: model.LedgerMutationReview, TargetID: "task", ReviewState: model.LedgerReviewAgreed, Rationale: "explicit owner control"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	root, identity := codingPlanWorkspaceAuthority(t, job)
	var generation int64
	if err := pool.QueryRow(ctx, `SELECT current_generation FROM jobs WHERE id=$1`, job.ID).Scan(&generation); err != nil {
		t.Fatal(err)
	}
	_, err = repository.ReplanJob(ctx, ReplanJobCommand{
		OperationID: codingPlanOperationID(t, "stale-ledger-replan", job.ID), JobID: job.ID,
		Feedback: "Amend the displayed proposal.", ObservedLedgerRevision: created.Revision,
		WorkspaceRoot: root, WorkspaceIdentity: identity,
	})
	if !errors.Is(err, ErrCodingLedgerRevision) {
		t.Fatalf("stale ledger replan at current revision %d error = %v", current.Revision, err)
	}
	var afterGeneration int64
	if err := pool.QueryRow(ctx, `SELECT current_generation FROM jobs WHERE id=$1`, job.ID).Scan(&afterGeneration); err != nil {
		t.Fatal(err)
	}
	if afterGeneration != generation {
		t.Fatalf("stale ledger replan advanced generation from %d to %d", generation, afterGeneration)
	}
}
