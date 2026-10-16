package queue

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func codingAgentTestConfiguration() codingagent.Configuration {
	route := codingagent.Route{Provider: "codex", Model: "fixture-model"}
	return codingagent.Configuration{Mode: codingagent.Agents, Planner: route, Executor: route, Reviewer: route}
}

func codingTaskLedgerCompletionJournalFixture() ([]codingagent.Entry, map[string]codingagent.Task) {
	config := codingAgentTestConfiguration()
	task := codingagent.Task{
		ID: "task-1", Goal: "Complete the selected ledger task",
		Acceptance: []string{"the selected task is fulfilled"},
		Checks:     []codingagent.Check{{ID: "task-check", Argv: []string{"go", "test"}}},
	}
	entries := []codingagent.Entry{}
	appendEntry := func(entry codingagent.Entry) int64 {
		entry.ID = int64(len(entries) + 1)
		entry.Generation = 1
		entries = append(entries, entry)
		return entry.ID
	}
	call := func(role codingagent.Role, thread string, content json.RawMessage) int64 {
		assignment := &codingagent.Assignment{
			JobID: 1, Generation: 1, Role: role, Task: &task, Root: "/workspace",
			Route: config.Executor,
		}
		if role == codingagent.Review {
			assignment.Route = config.Reviewer
		}
		callID := appendEntry(codingagent.Entry{Phase: "call_opened", Role: role, TaskID: task.ID, Assignment: assignment})
		appendEntry(codingagent.Entry{Phase: "native_event", Role: role, TaskID: task.ID, CallID: callID})
		appendEntry(codingagent.Entry{Phase: "call_completed", Role: role, TaskID: task.ID, CallID: callID, Result: &codingagent.Result{
			CallID: callID, Provider: assignment.Route.Provider, Model: assignment.Route.Model,
			Completed: true, ThreadID: thread, TurnID: thread + "-turn", Content: content,
		}})
		return callID
	}
	call(codingagent.Execute, "execution-thread", json.RawMessage(`{"questions":[]}`))
	review := call(codingagent.Review, "review-thread", json.RawMessage(`{"findings":[],"questions":[]}`))
	appendEntry(codingagent.Entry{Phase: "task_accepted", TaskID: task.ID, CallID: review, Checks: []codingagent.CheckResult{{ID: "task-check"}}})
	return entries, map[string]codingagent.Task{task.ID: task}
}

func TestCodingTaskLedgerCompletionRequiresExecutionThenIsolatedReview(t *testing.T) {
	entries, tasks := codingTaskLedgerCompletionJournalFixture()
	durablyAccepted := map[string]bool{"task-1": true}
	if err := validateCodingTaskLedgerCompletionJournal(entries, 1, codingAgentTestConfiguration(), tasks, durablyAccepted); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func([]codingagent.Entry) []codingagent.Entry
	}{
		{name: "missing execution", mutate: func(entries []codingagent.Entry) []codingagent.Entry { return entries[3:] }},
		{name: "shared execution review channel", mutate: func(entries []codingagent.Entry) []codingagent.Entry {
			for index := range entries {
				if entries[index].Phase == "call_completed" && entries[index].Role == codingagent.Review {
					entries[index].Result.ThreadID = "execution-thread"
				}
			}
			return entries
		}},
		{name: "foreign task context", mutate: func(entries []codingagent.Entry) []codingagent.Entry {
			for index := range entries {
				if entries[index].Phase == "call_opened" && entries[index].Role == codingagent.Review {
					entries[index].Assignment.Task.ID = "sibling"
				}
			}
			return entries
		}},
		{name: "reviewed after acceptance", mutate: func(entries []codingagent.Entry) []codingagent.Entry {
			return append(entries, codingagent.Entry{ID: 99, Generation: 1, Phase: "task_reviewed", TaskID: "task-1"})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate, _ := codingTaskLedgerCompletionJournalFixture()
			if err := validateCodingTaskLedgerCompletionJournal(test.mutate(candidate), 1, codingAgentTestConfiguration(), tasks, durablyAccepted); err == nil {
				t.Fatal("invalid task-ledger completion was accepted")
			}
		})
	}
}

func TestCodingTaskLedgerCompletionPreservesUntouchedDurableAcceptance(t *testing.T) {
	_, tasks := codingTaskLedgerCompletionJournalFixture()
	configuration := codingAgentTestConfiguration()
	if err := validateCodingTaskLedgerCompletionJournal(
		nil, 1, configuration, tasks, map[string]bool{"task-1": true},
	); err != nil {
		t.Fatalf("untouched carried acceptance: %v", err)
	}
	if err := validateCodingTaskLedgerCompletionJournal(
		nil, 1, configuration, tasks, map[string]bool{"task-1": false},
	); err == nil {
		t.Fatal("unfinished untouched task was accepted")
	}
	if err := validateCodingTaskLedgerCompletionJournal(
		[]codingagent.Entry{{Generation: 1, Phase: "task_executing", TaskID: "task-1"}},
		1, configuration, tasks, map[string]bool{"task-1": true},
	); err == nil {
		t.Fatal("current-generation execution bypassed its completion journal")
	}
}
