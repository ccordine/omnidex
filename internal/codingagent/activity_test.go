package codingagent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestControllerActivityFollowsDurableReceiptAndWorkspaceScope(t *testing.T) {
	for _, outsidePlan := range []bool{false, true} {
		controller, journal := fixtureController(t, fixtureWorkload(), func(Assignment) (any, error) { return "Task completed.", nil }, passingChecks)
		task := fixtureWorkload().Tasks[0]
		call := controller.assignment(Execute, &task, controller.Configuration.Executor)
		publications := 0
		controller.Activity = func(activity model.AgentActivity) {
			publications++
			if len(journal.entries) < 2 || journal.entries[len(journal.entries)-2].Phase != "native_event" || journal.entries[len(journal.entries)-1].Phase != "files_written" {
				t.Fatal("activity preceded its durable original receipt")
			}
		}
		controller.Providers[call.Route.Provider] = providerFunc(func(_ context.Context, call Assignment, event func(NativeEvent) error) (Result, error) {
			if err := event(NativeEvent{Frame: json.RawMessage(`{"native":"original"}`)}); err != nil {
				return Result{}, err
			}
			file := call.WriteFiles[0]
			if outsidePlan {
				file = "unplanned.txt"
			}
			activity := &model.AgentActivity{Schema: 1, Source: model.AgentActivityNative, Kind: model.AgentActivityFileEdit, Status: model.AgentActivityCompleted, ThreadID: "thread", TurnID: "turn", ItemID: "edit", Role: string(call.Role), TaskID: call.Task.ID, Paths: []string{file}}
			if err := event(NativeEvent{Activity: activity}); err != nil {
				return Result{}, err
			}
			return Result{Provider: call.Route.Provider, Model: call.Route.Model, ThreadID: "thread", TurnID: "turn", Completed: true, Content: json.RawMessage(`{}`)}, nil
		})
		_, err := controller.invoke(context.Background(), call, 1)
		if err != nil || publications != 1 {
			t.Fatalf("bound activity lost: publications=%d error=%v", publications, err)
		}
	}
}
