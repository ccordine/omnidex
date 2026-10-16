package queue

import (
	"context"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

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
		OwnerTurnDisposition: model.OwnerTurnDisposition{Kind: model.OwnerTurnRepresented, ItemIDs: ids},
	})
	if err != nil {
		t.Fatal(err)
	}
	current, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: created.LedgerID, OwnerTurnText: "Agree the objective.",
		Mutation: model.LedgerMutation{OperationID: "stale-replan-change", BaseRevision: created.Revision, OwnerTurnID: "stale-replan-change-turn",
			OwnerTurnDisposition: model.OwnerTurnDisposition{Kind: model.OwnerTurnRepresented, ItemIDs: []model.LedgerItemID{"objective"}},
			Operations:           []model.LedgerMutationOperation{{Kind: model.LedgerMutationReview, TargetID: "objective", ReviewState: model.LedgerReviewAgreed, Rationale: "explicit owner control"}}},
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

func TestCodingAgentFeedbackBindsObservedGenerationAndReplaysAfterReplan(t *testing.T) {
	pool, repository, job, claim, _ := freshAgentCodingClaim(t)
	ctx := context.Background()
	questions := []string{"Choose the approved first-generation detail."}
	if _, err := repository.CodingAgentEvidence(claim.Authority).Append(ctx, codingagent.Entry{Generation: 1, Phase: "questions", Questions: questions}); err != nil {
		t.Fatal(err)
	}
	if err := repository.PauseCodingAgentStep(ctx, claim.Authority, questions); err != nil {
		t.Fatal(err)
	}
	root, identity := codingPlanWorkspaceAuthority(t, job)
	command := SubmitJobFeedbackCommand{OperationID: codingPlanOperationID(t, "generation-bound-answer", job.ID), JobID: job.ID, Feedback: "Use the approved detail.", WorkspaceRoot: root, WorkspaceIdentity: identity}
	if _, err := repository.SubmitJobFeedback(ctx, command); err == nil {
		t.Fatal("Agents accepted feedback without an observed generation")
	}
	first := int64(1)
	command.ExpectedGeneration = &first
	command.ExpectedWaitingAttempt = &claim.Authority.Attempt
	accepted, err := repository.SubmitJobFeedback(ctx, command)
	if err != nil || !accepted.Applied {
		t.Fatalf("current generation answer: %#v %v", accepted, err)
	}
	currentLedger, exists, err := repository.CurrentCodingLedgerForJob(ctx, job.ID)
	if err != nil || !exists {
		t.Fatalf("current coding ledger: %#v %v", currentLedger, err)
	}
	if _, err := repository.ReplanJob(ctx, ReplanJobCommand{
		OperationID: codingPlanOperationID(t, "later-generation", job.ID), JobID: job.ID,
		Feedback: "Implement the next approved behavior.", ObservedLedgerRevision: currentLedger.Revision,
		WorkspaceRoot: root, WorkspaceIdentity: identity,
	}); err != nil {
		t.Fatal(err)
	}
	planner, err := repository.ClaimNextStep(ctx, "generation-two-plan-fixture")
	if err != nil || planner == nil {
		t.Fatalf("second planning: %v %v", planner, err)
	}
	secondProposal, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: currentLedger.LedgerID, OwnerTurnText: "Keep the exact approved task for generation two.",
		Mutation: model.LedgerMutation{
			OperationID: "generation-two-proposal", BaseRevision: currentLedger.Revision,
			OwnerTurnID: "generation-two-proposal-turn",
			OwnerTurnDisposition: model.OwnerTurnDisposition{
				Kind: model.OwnerTurnRepresented, ItemIDs: []model.LedgerItemID{"task"},
			},
			Operations: []model.LedgerMutationOperation{{
				Kind: model.LedgerMutationReview, TargetID: "task",
				ReviewState: model.LedgerReviewAgreed, Rationale: "retain the approved task",
			}},
		},
		CoveragePassed: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.MarkCodingLedgerReviewReady(ctx, planner.Authority, secondProposal.LedgerID, secondProposal.Revision); err != nil {
		t.Fatal(err)
	}
	secondAudit := repositoryReadyPlanningAudit(t, repository, secondProposal.LedgerID, secondProposal.Revision, "generation-two-freeze")
	secondFrozen, err := repository.FreezeCodingLedger(ctx, FreezeCodingLedgerCommand{
		LedgerID: secondProposal.LedgerID, Revision: secondProposal.Revision,
		OperationID: "generation-two-freeze", OwnerTurnID: "generation-two-freeze-turn",
		OwnerTurnText: "Freeze the generation-two task ledger.", AuditID: secondAudit,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CompleteCodingLedgerReview(ctx, job.ID, secondFrozen.LedgerID, secondFrozen.Revision); err != nil {
		t.Fatal(err)
	}
	executor, err := repository.ClaimNextStep(ctx, "generation-two-executor-fixture")
	if err != nil || executor == nil {
		t.Fatalf("second execution: %v %v", executor, err)
	}
	laterQuestions := []string{"Choose the approved second-generation detail."}
	if _, err := repository.CodingAgentEvidence(executor.Authority).Append(ctx, codingagent.Entry{Generation: 2, Phase: "questions", Questions: laterQuestions}); err != nil {
		t.Fatal(err)
	}
	if err := repository.PauseCodingAgentStep(ctx, executor.Authority, laterQuestions); err != nil {
		t.Fatal(err)
	}
	stale := command
	stale.OperationID = codingPlanOperationID(t, "stale-generation-answer", job.ID)
	if _, err := repository.SubmitJobFeedback(ctx, stale); !errors.Is(err, ErrJobFeedbackGeneration) {
		t.Fatalf("stale answer entered new generation: %v", err)
	}
	var secondAnswers int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM evidence WHERE job_id=$1 AND kind='coding_agent' AND payload_json->>'generation'='2' AND payload_json->>'phase'='answers'`, job.ID).Scan(&secondAnswers); err != nil || secondAnswers != 0 {
		t.Fatalf("stale answer changed durable state: %d %v", secondAnswers, err)
	}
	replay, err := repository.SubmitJobFeedback(ctx, command)
	if err != nil || replay.Applied || replay.Job.CurrentGeneration != 1 {
		t.Fatalf("exact historical generation replay: %#v %v", replay, err)
	}
	second := int64(2)
	command.ExpectedGeneration = &second
	if _, err := repository.SubmitJobFeedback(ctx, command); !errors.Is(err, ErrLifecycleOperationConflict) {
		t.Fatalf("old operation changed observed generation: %v", err)
	}
}
