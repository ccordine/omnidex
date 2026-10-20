package queue

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestPlanningAmendmentsReconnectFrozenBoundaryAndSignOff(t *testing.T) {
	pool, repo := freshUserIntentRepository(t)
	ctx := t.Context()
	command, fixture := userIntentFixture(t, repo, "Preserve the original instruction and seven task identities.")
	ledger := fixture.created
	original := ledger.Ledger.Tasks[0]
	operations := []model.LedgerMutationOperation{}
	ids := []model.LedgerItemID{original.ID}
	for i := 2; i <= 7; i++ {
		task := original
		task.ID = model.LedgerItemID(fmt.Sprintf("task%d", i))
		task.Rank = int64(i)
		task.Goal = fmt.Sprintf("Retain task %d.", i)
		ids = append(ids, task.ID)
		operations = append(operations, model.LedgerMutationOperation{Kind: model.LedgerMutationAdd, Item: &model.LedgerMutationItem{Task: &task}, Rationale: "Requested existing identity."})
	}
	var err error
	ledger, err = repo.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{LedgerID: ledger.LedgerID, OwnerTurnText: "Retain seven existing identities.", Mutation: model.LedgerMutation{OperationID: "seven", OwnerTurnID: "seven", BaseRevision: ledger.Revision, Operations: operations, OwnerTurnDisposition: model.RepresentedOwnerTurn(ids[1:])}})
	if err != nil {
		t.Fatal(err)
	}
	operations = nil
	for _, id := range ids {
		operations = append(operations, model.LedgerMutationOperation{Kind: model.LedgerMutationReview, TargetID: id, ReviewState: model.LedgerReviewAgreed, Rationale: "Owner agreement."})
	}
	ledger, err = repo.ApplyCodingLedgerControl(ctx, ApplyCodingLedgerMutationCommand{LedgerID: ledger.LedgerID, OwnerTurnText: "Agree all seven.", Mutation: model.LedgerMutation{OperationID: "agree-seven", OwnerTurnID: "agree-seven", BaseRevision: ledger.Revision, Operations: operations, OwnerTurnDisposition: model.RepresentedOwnerTurn(ids)}})
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := repo.FreezeCodingLedger(ctx, FreezeCodingLedgerCommand{LedgerID: ledger.LedgerID, Revision: ledger.Revision, OperationID: "start-seven", OwnerTurnID: "start-seven", OwnerTurnText: "Sign off and start."})
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.CompleteCodingLedgerReview(ctx, ledger.JobID, ledger.LedgerID, frozen.Revision); err != nil {
		t.Fatal(err)
	}
	claim, err := repo.ClaimNextStep(ctx, "amendment-execution")
	if err != nil || claim == nil {
		t.Fatal("claim", err)
	}
	selected, err := repo.NextIncompleteCodingLedgerTask(ctx, claim.Authority)
	if err != nil || selected == nil {
		t.Fatal("select", err)
	}
	ownerFile := filepath.Join(command.WorkspaceRoot, "owner-edit.txt")
	if err = os.WriteFile(ownerFile, []byte("preserved owner edit"), 0600); err != nil {
		t.Fatal(err)
	}
	messages := []ChannelSessionTurnCommand{}
	for i, text := range []string{"Add a requested task after the existing seven.", "Amend task2 after the current task.", "Explain the current task."} {
		id, _ := model.NewLifecycleOperationID()
		message := command
		message.OperationID = id
		message.Text = text
		message.ExpectedJobID = &claim.Job.ID
		message.ExpectedGeneration = &claim.Authority.Generation
		result, err := repo.SubmitChannelSessionTurn(ctx, message)
		if err != nil || !result.Applied || result.Disposition != model.ChannelSessionTurnQueued {
			t.Fatalf("intake%d: %+v %v", i, result, err)
		}
		if replay, err := repo.SubmitChannelSessionTurn(ctx, message); err != nil || replay.Applied {
			t.Fatal("duplicate intake", err)
		}
		messages = append(messages, message)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stored, _, err := readChannelSessionMessageTx(ctx, tx, messages[0].OperationID)
	if err != nil {
		t.Fatal(err)
	}
	forgedID, _ := model.NewLifecycleOperationID()
	stored.OperationID = forgedID
	stored.Command.OperationID = forgedID
	stored.PlanningBinding.Revision++
	raw, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = reserveLifecycleOperationIdentityTx(ctx, tx, forgedID, LifecycleChannelSessionMessage, raw); err == nil {
		t.Fatal("database accepted forged frozen binding")
	}
	tx.Rollback(ctx)
	stale := messages[0]
	stale.OperationID, _ = model.NewLifecycleOperationID()
	staleGeneration := claim.Authority.Generation + 1
	stale.ExpectedGeneration = &staleGeneration
	if _, err = repo.SubmitChannelSessionTurn(ctx, stale); !errors.Is(err, ErrStaleJobGeneration) {
		t.Fatal("stale generation admitted", err)
	}
	bad := messages[0]
	bad.Text = "changed replay"
	if _, err := repo.SubmitChannelSessionTurn(ctx, bad); !errors.Is(err, ErrLifecycleOperationConflict) {
		t.Fatal("changed replay", err)
	}
	bad = messages[0]
	bad.OperationID, _ = model.NewLifecycleOperationID()
	bad.WorkspaceIdentity = "foreign"
	if _, err := repo.SubmitChannelSessionTurn(ctx, bad); err == nil {
		t.Fatal("foreign session admitted")
	}
	for i, message := range messages {
		next, err := repo.NextChannelSessionMessage(ctx, claim.Authority)
		if err != nil || next == nil || next.OperationID != message.OperationID || next.PlanningBinding == nil || next.PlanningBinding.Revision != frozen.Revision || next.PlanningBinding.LedgerID != frozen.LedgerID {
			t.Fatalf("binding%d: %+v %v", i, next, err)
		}
		resolution := ResolveChannelSessionMessageCommand{Authority: claim.Authority, OperationID: message.OperationID, Kind: assemblyline.ObjectiveKindWorkspaceMutation}
		if i == 2 {
			resolution.Kind = assemblyline.ObjectiveKindAnswer
			resolution.Response = "The approved current task continues."
		}
		result, err := repo.ResolveChannelSessionMessage(ctx, resolution)
		if err != nil || result.Replanned || result.Job.CurrentGeneration != claim.Authority.Generation {
			t.Fatal("execution changed by pending input", err)
		}
		if replay, err := repo.ResolveChannelSessionMessage(ctx, resolution); err != nil || replay.Applied {
			t.Fatal("resolution replay", err)
		}
		if i < 2 {
			changed := resolution
			changed.Kind = assemblyline.ObjectiveKindAnswer
			changed.Response = "Different meaning"
			if _, err := repo.ResolveChannelSessionMessage(ctx, changed); !errors.Is(err, ErrLifecycleOperationConflict) {
				t.Fatal("queued meaning changed", err)
			}
		}
	}
	var amendmentCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM evidence WHERE job_id=$1 AND kind='coding_planning_amendment'`, claim.Job.ID).Scan(&amendmentCount); err != nil || amendmentCount != 2 {
		t.Fatal("duplicate queued evidence", amendmentCount, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE evidence SET payload_json='{}' WHERE kind='coding_planning_amendment' AND source_ref=$1`, messages[0].OperationID); err == nil {
		t.Fatal("queued authority was mutable")
	}
	reconnect := New(pool, repo.modelAuthority).WithCodingAgents(repo.codingAgents)
	pending, _, err := reconnect.CurrentCodingLedgerForJob(ctx, claim.Job.ID)
	if err != nil || len(pending.PendingAmendments) != 2 || pending.Revision != frozen.Revision || !reflect.DeepEqual(pending.Ledger, frozen.Ledger) {
		t.Fatal("pending/frozen separation", err)
	}
	again, err := reconnect.NextIncompleteCodingLedgerTask(ctx, claim.Authority)
	if err != nil || !reflect.DeepEqual(selected, again) {
		t.Fatal("frozen packet changed", err)
	}
	if next, err := reconnect.NextChannelSessionMessage(ctx, claim.Authority); err != nil || next != nil {
		t.Fatal("queued amendment redelivered to execution", err)
	}
	native := &recoveryRuntime{effects: map[codingagent.Role]int{}, parses: map[codingagent.Role]int{}}
	controller := codingagent.Controller{JobID: claim.Job.ID, Generation: claim.Authority.Generation, Root: command.WorkspaceRoot, Configuration: repo.codingAgents, Environment: native, Evidence: repo.CodingAgentEvidence(claim.Authority), Providers: map[string]codingagent.Provider{"codex": native}}
	if err = controller.RunTask(ctx, *selected); err != nil {
		t.Fatal(err)
	}
	if err = repo.AcceptCodingLedgerTask(ctx, claim.Authority, selected.ID); err != nil {
		t.Fatal(err)
	}
	if changed, err := repo.ReplanPendingCodingAmendments(ctx, claim.Authority); err != nil || !changed {
		t.Fatal("boundary replan", err)
	}
	if _, err = repo.NextChannelSessionMessage(ctx, claim.Authority); !errors.Is(err, ErrStaleStepAttempt) {
		t.Fatal("old consumer retained authority", err)
	}
	planner, err := reconnect.ClaimNextStep(ctx, "amendment-planner")
	if err != nil || planner == nil || planner.Job.ID != claim.Job.ID || planner.Job.Instruction != command.Text || planner.Authority.Generation != claim.Authority.Generation+1 {
		t.Fatal("same job replan", err)
	}
	for i := 0; i < 2; i++ {
		message, err := reconnect.NextChannelSessionMessage(ctx, planner.Authority)
		if err != nil || message == nil || message.OperationID != messages[i].OperationID || message.Command.WorkspaceIdentity != command.WorkspaceIdentity {
			t.Fatal("reconnect lost input", err)
		}
		current, _, err := reconnect.CurrentCodingLedgerForJob(ctx, claim.Job.ID)
		if err != nil {
			t.Fatal(err)
		}
		operationID := string(message.OperationID)
		if i == 0 {
			operationID = string(message.RedirectOperationID)
		}
		task := current.Ledger.Tasks[1]
		task.Goal = fmt.Sprintf("Requested amendment %d.", i)
		operation := model.LedgerMutationOperation{Kind: model.LedgerMutationAmend, TargetID: task.ID, Item: &model.LedgerMutationItem{Task: &task}, Rationale: "Requested amendment."}
		if i == 0 {
			task.ID = "task8"
			task.Rank = 8
			task.ReviewState = model.LedgerReviewProposed
			task.Goal = "Implement the requested addition."
			operation.Kind = model.LedgerMutationAdd
			operation.TargetID = ""
		}
		current, err = reconnect.ApplyCodingLedgerMutation(ctx, ApplyCodingLedgerMutationCommand{LedgerID: current.LedgerID, OwnerTurnText: message.Command.Text, Mutation: model.LedgerMutation{OperationID: operationID, OwnerTurnID: string(message.OperationID), BaseRevision: current.Revision, OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{task.ID}), Operations: []model.LedgerMutationOperation{operation}}})
		if err != nil {
			t.Fatal("same ledger amendment", err)
		}
		if _, err = reconnect.CodingAgentEvidence(planner.Authority).Append(ctx, codingagent.Entry{Generation: planner.Authority.Generation, Phase: "planning_response", Role: codingagent.Plan, PlanningProposal: &current.Ledger, Notes: []string{string(message.OperationID), message.Command.Text, "Persisted requested amendment."}}); err != nil {
			t.Fatal(err)
		}
		if _, err = reconnect.ResolvePlanningSessionMessage(ctx, planner.Authority, message.OperationID); err != nil {
			t.Fatal("planning receipt", err)
		}
		if _, err = reconnect.ResolvePlanningSessionMessage(ctx, planner.Authority, message.OperationID); err != nil {
			t.Fatal("planning replay", err)
		}
	}
	proposed, _, err := reconnect.CurrentCodingLedgerForJob(ctx, claim.Job.ID)
	if err != nil || proposed.LedgerID != frozen.LedgerID || len(proposed.PendingAmendments) != 0 || len(proposed.Ledger.Tasks) != 8 {
		t.Fatal("proposal", err)
	}
	if err = reconnect.MarkCodingLedgerReviewReady(ctx, planner.Authority, proposed.LedgerID, proposed.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err = reconnect.FreezeCodingLedger(ctx, FreezeCodingLedgerCommand{LedgerID: proposed.LedgerID, Revision: proposed.Revision, OperationID: "unagreed", OwnerTurnID: "unagreed", OwnerTurnText: "Start."}); err == nil {
		t.Fatal("amendment bypassed agreement")
	}
	taskID := proposed.Ledger.Tasks[1].ID
	proposed, err = reconnect.ApplyCodingLedgerControl(ctx, ApplyCodingLedgerMutationCommand{LedgerID: proposed.LedgerID, OwnerTurnText: "Agree requested amendment.", Mutation: model.LedgerMutation{OperationID: "agree-amendment", OwnerTurnID: "agree-amendment", BaseRevision: proposed.Revision, OwnerTurnDisposition: model.RepresentedOwnerTurn([]model.LedgerItemID{taskID, "task8"}), Operations: []model.LedgerMutationOperation{{Kind: model.LedgerMutationReview, TargetID: taskID, ReviewState: model.LedgerReviewAgreed, Rationale: "Explicit owner agreement."}, {Kind: model.LedgerMutationReview, TargetID: "task8", ReviewState: model.LedgerReviewAgreed, Rationale: "Explicit owner agreement to addition."}}}})
	if err != nil {
		t.Fatal(err)
	}
	refrozen, err := reconnect.FreezeCodingLedger(ctx, FreezeCodingLedgerCommand{LedgerID: proposed.LedgerID, Revision: proposed.Revision, OperationID: "sign-amendments", OwnerTurnID: "sign-amendments", OwnerTurnText: "Sign off these amendments."})
	if err != nil {
		t.Fatal(err)
	}
	if err = reconnect.CompleteCodingLedgerReview(ctx, claim.Job.ID, refrozen.LedgerID, refrozen.Revision); err != nil {
		t.Fatal(err)
	}
	var state string
	if err = pool.QueryRow(ctx, `SELECT state FROM coding_agent_tasks WHERE ledger_id=$1 AND frozen_revision=$2 AND task_item_id=$3`, refrozen.LedgerID, refrozen.Revision, selected.ID).Scan(&state); err != nil || state != "accepted" {
		t.Fatal("accepted work reopened", state, err)
	}
	for i, task := range refrozen.Ledger.Tasks[:7] {
		if task.ID != ids[i] {
			t.Fatal("task identity changed")
		}
	}
	if data, err := os.ReadFile(ownerFile); err != nil || string(data) != "preserved owner edit" {
		t.Fatal("checkout edit lost", err)
	}
}

func TestPlanningAmendmentsRetireAcrossReplanOnCancellation(t *testing.T) {
	_, repo := freshUserIntentRepository(t)
	ctx := t.Context()
	command, fixture := userIntentFixture(t, repo, "Preserve queued input until an explicit transition.")
	start, _ := saveIntent(t, repo, command, fixture.created.Revision, "Approve and start.", statePatch("/items/0/review_state", "agreed"), statePatch("/plan_state", "frozen"))
	if _, err := repo.ExecuteUserIntent(ctx, start.OperationID, command.ChannelID, command.WorkspaceIdentity); err != nil {
		t.Fatal(err)
	}
	claim, err := repo.ClaimNextStep(ctx, "amendment-control")
	if err != nil || claim == nil {
		t.Fatal(err)
	}
	current, _, err := repo.CurrentCodingLedgerForJob(ctx, claim.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	message := command
	message.OperationID, _ = model.NewLifecycleOperationID()
	message.ExpectedJobID = &claim.Job.ID
	message.ExpectedGeneration = &claim.Authority.Generation
	message.Text = "Queue this future amendment."
	if _, err = repo.SubmitChannelSessionTurn(ctx, message); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.ResolvePlanningSessionMessage(ctx, claim.Authority, message.OperationID); !errors.Is(err, ErrCodingLedgerState) {
		t.Fatal("frozen input entered planning without transition", err)
	}
	if _, err = repo.ResolveChannelSessionMessage(ctx, ResolveChannelSessionMessageCommand{Authority: claim.Authority, OperationID: message.OperationID, Kind: assemblyline.ObjectiveKindWorkspaceMutation}); err != nil {
		t.Fatal(err)
	}
	operation, _ := model.NewLifecycleOperationID()
	replanned, err := repo.ReplanJob(ctx, ReplanJobCommand{OperationID: operation, JobID: claim.Job.ID, ObservedLedgerRevision: current.Revision, Feedback: "Explicit owner transition to revise the same job.", WorkspaceRoot: command.WorkspaceRoot, WorkspaceIdentity: command.WorkspaceIdentity})
	if err != nil {
		t.Fatal(err)
	}
	operation, _ = model.NewLifecycleOperationID()
	if _, err = repo.CancelJob(ctx, CancelJobCommand{OperationID: operation, JobID: replanned.Job.ID, Reason: "Cancel the same job and its pending input.", WorkspaceRoot: command.WorkspaceRoot, WorkspaceIdentity: command.WorkspaceIdentity}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := repo.ChannelSessionSnapshot(ctx, command.ChannelID, MaxChannelSessionMessages, command.WorkspaceIdentity)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, turn := range snapshot.Turns {
		if turn.OperationID == message.OperationID {
			found = true
			if turn.MessageState != model.ChannelSessionMessageRetired {
				t.Fatal("cancelled amendment remained pending", turn)
			}
		}
	}
	if !found {
		t.Fatal("retirement lost original input provenance")
	}
}
