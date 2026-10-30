package codingagent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type durableTaskEvidence struct{ entries []Entry }

func (store *durableTaskEvidence) Append(_ context.Context, entry Entry) (int64, error) {
	entry.ID = int64(len(store.entries) + 1)
	store.entries = append(store.entries, entry)
	return entry.ID, nil
}
func (store *durableTaskEvidence) Read(context.Context) ([]Entry, error) { return store.entries, nil }
func (store *durableTaskEvidence) ReadTaskClarifications(_ context.Context, taskID string) ([]TaskClarification, error) {
	return ExtractTaskClarifications(store.entries, taskID), nil
}

type durableTaskEnvironment struct{}

func (durableTaskEnvironment) Attest(context.Context) error { return nil }

type durableTaskProvider struct {
	t     *testing.T
	task  Task
	calls int
}

func (provider *durableTaskProvider) Run(_ context.Context, call Assignment, event func(NativeEvent) error) (Result, error) {
	provider.calls++
	content := json.RawMessage(`"Task completed."`)
	switch call.Role {
	case Execute:
		if call.Session != nil || call.Task == nil || call.Task.Goal != provider.task.Goal || len(call.Clarifications) != 1 || call.Clarifications[0].Answer != "CSV" {
			provider.t.Fatalf("execution was not reconstructed from durable task state: %+v", call)
		}
		raw, _ := json.Marshal(PromptContext(call))
		if strings.Contains(string(raw), "other-task") || strings.Contains(string(raw), "approve-command") {
			provider.t.Fatal("execution inherited unrelated task or permission history")
		}
	case Review:
		if call.Session != nil || call.Task == nil || call.Task.ID != provider.task.ID {
			provider.t.Fatal("review was not independent and task-local")
		}
	case ExecutionResponse:
		content = json.RawMessage(`{"kind":"done"}`)
	case ReviewResponse:
		content = json.RawMessage(`{"kind":"done","findings":[],"verification":"Verified the exact selected task."}`)
	}
	if err := event(NativeEvent{Frame: json.RawMessage(`{"fixture":"durable-state"}`)}); err != nil {
		return Result{}, err
	}
	return Result{Provider: call.Route.Provider, Model: call.Route.Model, Completed: true,
		ThreadID: "fresh-thread", TurnID: "fresh-turn", Content: content}, nil
}

func TestTaskRestoresDurableAnswersWithoutNativeConversation(t *testing.T) {
	for _, goal := range []string{"Produce the requested temperature report.", "Convert the requested configuration field."} {
		t.Run(goal, func(t *testing.T) {
			task := Task{ID: "selected-task", Goal: goal}
			store := &durableTaskEvidence{entries: []Entry{
				{Phase: "execution_session", TaskID: task.ID, Session: &ExecutionSession{ThreadID: "old-thread", TurnID: "old-turn", State: "idle"}},
				{Phase: "execution_response", TaskID: task.ID, CallID: 11, Notes: []string{"Which format?"}, ResponseKind: ExecutionQuestion},
				{Phase: "execution_reply", TaskID: task.ID, CallID: 11, Notes: []string{"reply-id", "CSV"}},
				{Phase: "execution_native_input", TaskID: task.ID, CallID: 12, Notes: []string{"approve-command"}, NativeInput: &NativeInputRequest{Method: "item/commandExecution/requestApproval"}},
				{Phase: "execution_reply", TaskID: task.ID, CallID: 12, Notes: []string{"permission-id", "approve"}, NativeInput: &NativeInputRequest{Method: "item/commandExecution/requestApproval"}},
				{Phase: "execution_response", TaskID: "other-task", CallID: 13, Notes: []string{"other-task question"}, ResponseKind: ExecutionQuestion},
			}}
			provider := &durableTaskProvider{t: t, task: task}
			route := Route{Provider: "fixture", Model: "fixture", Effort: "low", ServiceTier: "default"}
			controller := Controller{JobID: 1, Generation: 1, Root: t.TempDir(), Evidence: store, Environment: durableTaskEnvironment{},
				Providers: map[string]Provider{"fixture": provider}, Configuration: Configuration{Mode: Agents, Planner: route, Executor: route, Reviewer: route, LedgerParser: route}}
			if err := controller.RunTask(t.Context(), task); err != nil {
				t.Fatal(err)
			}
			calls := provider.calls
			// A new controller sees acceptance in the evidence store and performs
			// no repeated execution or review, regardless of native history.
			restarted := controller
			if err := restarted.RunTask(t.Context(), task); err != nil || provider.calls != calls {
				t.Fatalf("restart replayed accepted work: calls=%d want=%d error=%v", provider.calls, calls, err)
			}
		})
	}
}
