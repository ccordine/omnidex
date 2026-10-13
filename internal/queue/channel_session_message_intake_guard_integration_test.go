package queue

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestActiveSessionMessageIntakeRejectsNullOperationIdentities(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	_, question := activeSessionMessageFixture(t, repository)
	for _, field := range []string{"operation_id", "command,operation_id"} {
		t.Run(field, func(t *testing.T) {
			id := codingPlanOperationID(t, "null-message-identity", 1)
			_, err := pool.Exec(ctx, `INSERT INTO lifecycle_operation_registry(operation_id,kind,command_payload)
			 SELECT $1,'channel_session_message',jsonb_set(
			   jsonb_set(jsonb_set(command_payload,'{operation_id}',to_jsonb($1::text)),
			   '{command,operation_id}',to_jsonb($1::text)),$2::text[],'null'::jsonb)
			 FROM lifecycle_operation_registry WHERE operation_id=$3`, id, "{"+field+"}", question.OperationID)
			if err == nil {
				t.Fatal("SQL admitted an operation identity that the authoritative reader cannot restore")
			}
		})
	}
}

func TestActiveSessionMessageReplyCannotBeDeletedOrChanged(t *testing.T) {
	for _, action := range []string{"delete", "update"} {
		t.Run(action, func(t *testing.T) {
			pool, repository := freshCodingPlanRepository(t)
			ctx := context.Background()
			claim, question := activeSessionMessageFixture(t, repository)
			result, err := repository.ResolveChannelSessionMessage(ctx, ResolveChannelSessionMessageCommand{Authority: claim.Authority, OperationID: question.OperationID, Kind: assemblyline.ObjectiveKindAnswer, Response: "The original durable answer."})
			if err != nil {
				t.Fatal(err)
			}
			query := `DELETE FROM ai_channel_messages WHERE id=$1`
			if action == "update" {
				query = `UPDATE ai_channel_messages SET content='Changed historical answer.' WHERE id=$1`
			}
			if _, err := pool.Exec(ctx, query, result.ResponseMessage.ID); err == nil {
				t.Fatalf("%s corrupted the reply under an immutable terminal receipt", action)
			}
		})
	}
}

func TestActiveSessionMessageRedirectRequiresExactDesignatedReplan(t *testing.T) {
	for _, defect := range []string{"wrong control kind", "changed feedback"} {
		t.Run(defect, func(t *testing.T) {
			pool, repository := freshCodingPlanRepository(t)
			ctx := context.Background()
			claim, question := activeSessionMessageFixture(t, repository)
			intake, err := repository.NextChannelSessionMessage(ctx, claim.Authority)
			if err != nil || intake == nil {
				t.Fatalf("read designated child: %#v %v", intake, err)
			}
			var control LifecycleJobResult
			if defect == "wrong control kind" {
				control, err = repository.CancelJob(ctx, CancelJobCommand{OperationID: intake.RedirectOperationID, JobID: claim.Job.ID, Reason: "Stop this fixture.", WorkspaceRoot: question.WorkspaceRoot, WorkspaceIdentity: question.WorkspaceIdentity})
			} else {
				control, err = repository.ReplanJob(ctx, ReplanJobCommand{OperationID: intake.RedirectOperationID, JobID: claim.Job.ID, Feedback: "A different redirection.", WorkspaceRoot: question.WorkspaceRoot, WorkspaceIdentity: question.WorkspaceIdentity})
			}
			if err != nil {
				t.Fatal(err)
			}
			payload, err := json.Marshal(channelSessionMessageResolution{OperationID: question.OperationID, Kind: "workspace_mutation", Job: control.Job, Authority: &claim.Authority, ControlOperationID: intake.RedirectOperationID})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := pool.Exec(ctx, `INSERT INTO evidence(job_id,step_id,kind,source_type,source_ref,payload_json)
			 VALUES($1,$2,'channel_session_message','lifecycle_operation',$3,$4::jsonb)`, claim.Job.ID, claim.Step.ID, question.OperationID, string(payload)); err == nil {
				t.Fatalf("%s backed a false workspace mutation receipt", defect)
			}
		})
	}
}
