package queue

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestUserIntentCurrentNativeChoiceAndReceiptReplay(t *testing.T) {
	for _, label := range []string{"allow once", "deny"} {
		t.Run(label, func(t *testing.T) {
			pool, repo := freshUserIntentRepository(t)
			ctx := t.Context()
			command, fixture := userIntentFixture(t, repo, "Answer the exact current native request.")
			start, _ := saveIntent(t, repo, command, fixture.created.Revision, "Approve and start.", statePatch("/items/0/review_state", "agreed"), statePatch("/plan_state", "frozen"))
			started, err := repo.ExecuteUserIntent(ctx, start.OperationID, command.ChannelID, command.WorkspaceIdentity)
			if err != nil {
				t.Fatal(err)
			}
			executor, err := repo.ClaimNextStep(ctx, "intent-native")
			if err != nil || executor == nil {
				t.Fatalf("executor: %v", err)
			}
			task, err := repo.NextIncompleteCodingLedgerTask(ctx, executor.Authority)
			if err != nil || task == nil {
				t.Fatalf("task: %v", err)
			}
			callID, err := repo.CodingAgentEvidence(executor.Authority).Append(ctx, codingagent.Entry{Generation: executor.Job.CurrentGeneration, Phase: "call_opened", Role: codingagent.Execute, TaskID: task.ID})
			if err != nil {
				t.Fatal(err)
			}
			choices := []codingagent.NativeInputChoice{
				{Label: "allow once", Response: json.RawMessage(`{"decision":"accept"}`)},
				{Label: "deny", Response: json.RawMessage(`{"decision":"decline"}`)},
			}
			message := command
			message.OperationID, _ = model.NewLifecycleOperationID()
			message.ExpectedJobID = &executor.Job.ID
			message.ExpectedGeneration = &executor.Authority.Generation
			message.Text = "Queue a future change while the current native request stays authoritative."
			if _, err := repo.SubmitChannelSessionTurn(ctx, message); err != nil {
				t.Fatal(err)
			}
			inputID, err := repo.WaitForCodingNativeInput(ctx, executor.Authority, codingagent.NativeInputRequest{ID: json.RawMessage(`1`), Method: "item/commandExecution/requestApproval", ThreadID: "synthetic-thread", TurnID: "synthetic-turn", Prompt: "Allow the current synthetic command?", Choices: choices}, callID, task.ID)
			if err != nil {
				t.Fatal(err)
			}
			if next, err := repo.NextChannelSessionMessage(ctx, executor.Authority); err != nil || next != nil {
				t.Fatal("message poller cancelled native input wait", err)
			}
			if result, err := repo.ResolveChannelSessionMessage(ctx, ResolveChannelSessionMessageCommand{Authority: executor.Authority, OperationID: message.OperationID, Kind: assemblyline.ObjectiveKindWorkspaceMutation}); err != nil || result.Replanned {
				t.Fatal("in-flight amendment disrupted native input", err)
			}
			index := 0
			if label == "deny" {
				index = 1
			}
			request, result := saveIntent(t, repo, command, started.Revision, "Use the selected answer.", statePatch("/native_reply", label))
			if result.RequiresApproval {
				t.Fatal("native choice requested recursive approval")
			}
			receipt, err := repo.ExecuteUserIntent(ctx, request.OperationID, command.ChannelID, command.WorkspaceIdentity)
			if err != nil {
				t.Fatal(err)
			}
			reply, found, err := repo.CodingNativeInputReply(ctx, executor.Authority, inputID)
			if err != nil || !found || !sameCodingLedgerJSON(reply, choices[index].Response) {
				t.Fatalf("reply=%s found=%t err=%v", reply, found, err)
			}
			replay, err := repo.ExecuteUserIntent(ctx, request.OperationID, command.ChannelID, command.WorkspaceIdentity)
			if err != nil || !reflect.DeepEqual(receipt, replay) {
				t.Fatalf("replay=%+v %v", replay, err)
			}
			var count int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM evidence WHERE job_id=$1 AND payload_json->>'phase'='execution_reply'`, executor.Job.ID).Scan(&count); err != nil || count != 1 {
				t.Fatalf("reply effects=%d %v", count, err)
			}
		})
	}
}
