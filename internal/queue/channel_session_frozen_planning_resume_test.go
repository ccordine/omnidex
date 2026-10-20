package queue

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestInterruptedFrozenPlanningResumesThroughRevisionBoundControl(t *testing.T) {
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
	// Frozen execution interruption is no longer an unbound planning turn.
	// Its explicit recovery control retains the exact generation and revision.
	if _, err := repository.SubmitChannelSessionTurn(ctx, resume); !errors.Is(err, ErrStepNotWritable) {
		t.Fatalf("unbound frozen resume was admitted: %v", err)
	}
	paused, err := repository.CurrentJobDetails(ctx, initial.Job.ID)
	if err != nil || paused.Job.Status != model.JobStatusWaiting || paused.Job.CurrentGeneration != interrupted.Job.CurrentGeneration {
		t.Fatal("rejected turn changed paused authority", paused, err)
	}
	control := ResumeJobCommand{OperationID: codingPlanOperationID(t, "resume-frozen-control", initial.Job.ID), JobID: initial.Job.ID, ExpectedGeneration: interrupted.Job.CurrentGeneration, ObservedLedgerRevision: frozen.Revision, Feedback: resume.Text, WorkspaceRoot: command.WorkspaceRoot, WorkspaceIdentity: command.WorkspaceIdentity}
	continued, err := repository.ResumeJob(ctx, control)
	if err != nil || !continued.Applied || continued.Job.ID != initial.Job.ID || continued.Job.CurrentGeneration != interrupted.Job.CurrentGeneration {
		t.Fatal("revision-bound resume lost frozen authority", continued, err)
	}
	assertSessionCodingConfiguration(t, continued.Job, configuration)
	current, exists, err := repository.CurrentCodingLedgerForJob(ctx, initial.Job.ID)
	if err != nil || !exists || current.State != "frozen" || current.Revision != frozen.Revision || !reflect.DeepEqual(current.Ledger, frozen.Ledger) {
		t.Fatalf("resume silently changed the approved ledger: %+v %v", current, err)
	}
	replayed, err := repository.ResumeJob(ctx, control)
	if err != nil || replayed.Applied || replayed.Job.ID != continued.Job.ID || replayed.Job.CurrentGeneration != continued.Job.CurrentGeneration {
		t.Fatalf("revision-bound resume replayed twice: %+v %v", replayed, err)
	}
}
