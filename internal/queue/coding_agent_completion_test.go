package queue

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func codingAgentTestConfiguration() codingagent.Configuration {
	route := codingagent.Route{Provider: "codex", Model: "fixture-model"}
	return codingagent.Configuration{Mode: codingagent.Agents, Planner: route, LedgerParser: route, Executor: route, Reviewer: route}
}

func codingTaskLedgerCompletionJournalFixture() ([]codingagent.Entry, map[string]codingagent.Task) {
	config := codingAgentTestConfiguration()
	task := codingagent.Task{
		ID: "task-1", Goal: "Complete the selected ledger task",
		Acceptance: []model.TaskAcceptance{{ID: "task-check", Text: "the selected task is fulfilled", Verification: []string{"go test"}}},
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
		if role == codingagent.ReviewResponse {
			assignment.Route, assignment.Response = config.LedgerParser, "Checked the exact task; no defects found."
		}
		callID := appendEntry(codingagent.Entry{Phase: "call_opened", Role: role, TaskID: task.ID, Assignment: assignment})
		appendEntry(codingagent.Entry{Phase: "native_event", Role: role, TaskID: task.ID, CallID: callID})
		appendEntry(codingagent.Entry{Phase: "call_completed", Role: role, TaskID: task.ID, CallID: callID, Result: &codingagent.Result{
			CallID: callID, Provider: assignment.Route.Provider, Model: assignment.Route.Model,
			Completed: true, ThreadID: thread, TurnID: thread + "-turn", Content: content,
		}})
		return callID
	}
	execution := call(codingagent.Execute, "execution-thread", json.RawMessage(`"Task completed."`))
	appendEntry(codingagent.Entry{Phase: "execution_response", TaskID: task.ID, CallID: execution,
		ResponseKind: codingagent.ExecutionDone, Notes: []string{"Task completed."}})
	review := call(codingagent.Review, "review-thread", json.RawMessage(`"Checked the exact task; no defects found."`))
	parsed := codingagent.ReviewResponseResult{Kind: codingagent.ExecutionDone, ReviewResult: codingagent.ReviewResult{Findings: []codingagent.Finding{}, Verification: "checked exact task"}}
	content, _ := json.Marshal(parsed)
	parser := call(codingagent.ReviewResponse, "review-parser-thread", content)
	appendEntry(codingagent.Entry{Phase: "review_response", TaskID: task.ID, CallID: review, ParserCallID: parser,
		Role: codingagent.Review, ResponseKind: codingagent.ExecutionDone, ReviewResponse: &parsed, Notes: []string{"Checked the exact task; no defects found."}})
	appendEntry(codingagent.Entry{Phase: "task_accepted", TaskID: task.ID, CallID: review})
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
		{name: "missing execution", mutate: func(entries []codingagent.Entry) []codingagent.Entry { return entries[4:] }},
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
		{name: "execution question routing", mutate: func(entries []codingagent.Entry) []codingagent.Entry {
			return append(entries, codingagent.Entry{ID: 99, Generation: 1, Phase: "questions"})
		}},
		{name: "executor authored result field", mutate: func(entries []codingagent.Entry) []codingagent.Entry {
			for index := range entries {
				if entries[index].Phase == "call_completed" && entries[index].Role == codingagent.Execute {
					entries[index].Result.Content = json.RawMessage(`{"questions":[]}`)
				}
			}
			return entries
		}},
		{name: "reviewer authored question field", mutate: func(entries []codingagent.Entry) []codingagent.Entry {
			for index := range entries {
				if entries[index].Phase == "call_completed" && entries[index].Role == codingagent.Review {
					entries[index].Result.Content = json.RawMessage(`{"findings":[],"questions":[],"verification":"checked"}`)
				}
			}
			return entries
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
