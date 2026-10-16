package queue

import (
	"context"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestInterruptedFrozenPlanningResumesThroughOrdinarySessionTurn(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	configuration := codingAgentTestConfiguration()
	repository.WithCodingAgents(configuration)
	ctx := context.Background()
	command := channelSessionCodingCommand(t, repository)
	command.CodingMode = codingagent.Agents
	initial, err := repository.SubmitChannelSessionTurn(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	planner, err := repository.ClaimNextStep(ctx, "frozen-planning-fixture")
	if err != nil || planner == nil || planner.Step.Action != "v3_coding_plan" {
		t.Fatalf("claim planner: %+v %v", planner, err)
	}
	fixture := codingAgentPlanReviewFixture(t, repository, planner, "Verify a retained planning session.")
	operations := make([]model.LedgerMutationOperation, 0, len(fixture.itemIDs))
	for _, itemID := range fixture.itemIDs {
		operations = append(operations, model.LedgerMutationOperation{Kind: model.LedgerMutationReview,
			TargetID: itemID, ReviewState: model.LedgerReviewAgreed, Rationale: "fixture owner agreed"})
	}
	created, err := repository.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{
		LedgerID: fixture.created.LedgerID, OwnerTurnText: fixture.goal,
		Mutation: model.LedgerMutation{OperationID: "agree-resume", BaseRevision: fixture.created.Revision,
			OwnerTurnID: "owner-agree-resume", OwnerTurnDisposition: model.RepresentedOwnerTurn(fixture.itemIDs), Operations: operations},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.MarkCodingLedgerReviewReady(ctx, planner.Authority, created.LedgerID, created.Revision); err != nil {
		t.Fatal(err)
	}
	frozen, err := repository.FreezeCodingLedger(ctx, FreezeCodingLedgerCommand{LedgerID: created.LedgerID, Revision: created.Revision,
		OperationID: "freeze-resume-fixture", OwnerTurnID: "owner-freeze-resume", OwnerTurnText: "Freeze this reviewed fixture."})
	if err != nil {
		t.Fatal(err)
	}
	resume := command
	resume.OperationID = codingPlanOperationID(t, "uninterrupted-frozen-turn", initial.Job.ID)
	resume.Text = "Continue."
	if _, err := repository.SubmitChannelSessionTurn(ctx, resume); !errors.Is(err, ErrChannelSessionTurnInvalid) {
		t.Fatalf("frozen planning without an interrupt bypassed its normal transition: %v", err)
	}
	if err := repository.CompleteCodingLedgerReview(ctx, initial.Job.ID, frozen.LedgerID, frozen.Revision); err != nil {
		t.Fatal(err)
	}
	work, err := repository.ClaimNextStep(ctx, "frozen-work-fixture")
	if err != nil || work == nil || work.Step.Action != "v3_coding" {
		t.Fatalf("claim approved work: %+v %v", work, err)
	}
	interrupted, err := repository.InterruptJob(ctx, ReplanJobCommand{OperationID: codingPlanOperationID(t, "pause-frozen-planner", initial.Job.ID),
		JobID: initial.Job.ID, Feedback: "Pause the planning action.", WorkspaceRoot: command.WorkspaceRoot, WorkspaceIdentity: command.WorkspaceIdentity})
	if err != nil {
		t.Fatal(err)
	}
	resume.OperationID = codingPlanOperationID(t, "resume-frozen-planner", initial.Job.ID)
	continued, err := repository.SubmitChannelSessionTurn(ctx, resume)
	if err != nil || !continued.Applied || continued.Job.ID != initial.Job.ID || continued.Job.CurrentGeneration != interrupted.Job.CurrentGeneration+1 {
		t.Fatalf("ordinary resume lost saved planning authority: %+v %v", continued, err)
	}
	assertSessionCodingConfiguration(t, continued.Job, configuration)
	continuity, err := repository.ObjectiveContinuityAuthorities(ctx, continued.Job, replanCodingBoundary)
	if err != nil || continuity.Replan == nil || continuity.ObservedLedgerRevision != frozen.Revision || continuity.Replan.Feedback != resume.Text {
		t.Fatalf("resume did not bind the retained frozen revision: %+v %v", continuity, err)
	}
	current, exists, err := repository.CurrentCodingLedgerForJob(ctx, initial.Job.ID)
	if err != nil || !exists || current.State != "frozen" || current.Revision != frozen.Revision {
		t.Fatalf("resume silently changed the approved ledger: %+v %v", current, err)
	}
	replayed, err := repository.SubmitChannelSessionTurn(ctx, resume)
	if err != nil || replayed.Applied || replayed.Job.ID != continued.Job.ID || replayed.Job.CurrentGeneration != continued.Job.CurrentGeneration {
		t.Fatalf("ordinary resume replayed twice: %+v %v", replayed, err)
	}
}
