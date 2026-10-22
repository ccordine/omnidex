package queue

import (
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/sessionintent"
)

func TestUserIntentReceiptsIdentifyPersistedEchoes(t *testing.T) {
	for _, kind := range []string{"message", "interrupt", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			pool, repo := freshUserIntentRepository(t)
			command, fixture := userIntentFixture(t, repo, "Keep a small local report.")
			patch := statePatch("/message", sessionintent.Content{Text: "Keep this exact note.", Recipient: "planner"})
			if kind == "interrupt" {
				start, _ := saveIntent(t, repo, command, fixture.created.Revision, "Start.", statePatch("/items/0/review_state", "agreed"), statePatch("/plan_state", "frozen"), statePatch("/job_status", "running"))
				if _, err := repo.ExecuteUserIntent(t.Context(), start.OperationID, command.ChannelID, command.WorkspaceIdentity); err != nil {
					t.Fatal(err)
				}
				patch = statePatch("/job_status", model.JobStatusWaiting)
			} else if kind == "cancel" {
				patch = statePatch("/job_status", model.JobStatusCanceled)
			}
			current, _, err := repo.CurrentCodingLedgerForJob(t.Context(), fixture.created.JobID)
			if err != nil {
				t.Fatal(err)
			}
			request, result := saveIntent(t, repo, command, current.Revision, "Keep this exact note.", patch)
			if result.RequiresApproval {
				request, _ = saveIntent(t, repo, command, current.Revision, "Yes, do it.", statePatch("/approval", true))
			}
			receipt, err := repo.ExecuteUserIntent(t.Context(), request.OperationID, command.ChannelID, command.WorkspaceIdentity)
			if err != nil {
				t.Fatal(err)
			}
			identity := receipt.TurnOperationID
			if kind != "message" {
				identity = receipt.ControlOperationID
			}
			if identity == "" {
				t.Fatal("committed operation omitted its transcript identity")
			}
			var found bool
			if err := pool.QueryRow(t.Context(), `SELECT EXISTS(SELECT 1 FROM lifecycle_operation_registry WHERE operation_id=$1)`, identity).Scan(&found); err != nil || !found {
				t.Fatal("receipt did not identify a persisted operation", err)
			}
			replay, err := repo.ExecuteUserIntent(t.Context(), request.OperationID, command.ChannelID, command.WorkspaceIdentity)
			if err != nil || !reflect.DeepEqual(receipt, replay) {
				t.Fatal("replay changed transcript identities", err)
			}
		})
	}
}
