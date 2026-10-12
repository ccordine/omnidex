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

func codingAgentCompletionJournalFixture() []codingagent.Entry {
	config := codingAgentTestConfiguration()
	workload := codingagent.Workload{Tasks: []codingagent.Task{{ID: "task-1", Goal: "Complete approved behavior", Acceptance: []string{"Approved behavior works"}, RequirementIDs: []string{"requirement-1"}, ReadFiles: []string{}, WriteFiles: []string{"feature.go"}, Dependencies: []string{}, Checks: []codingagent.Check{{ID: "task-check", Argv: []string{"go", "test"}}}}}, Checks: []codingagent.Check{{ID: "overall-check", Argv: []string{"go", "test"}}}}
	entries := []codingagent.Entry{}
	appendEntry := func(entry codingagent.Entry) int64 {
		entry.ID = int64(len(entries) + 1)
		entry.Generation = 1
		entries = append(entries, entry)
		return entry.ID
	}
	call := func(role codingagent.Role, task string) int64 {
		route := config.Reviewer
		if role == codingagent.Plan {
			route = config.Planner
		}
		assignment := &codingagent.Assignment{JobID: 1, Generation: 1, Role: role, Route: route, Requirements: []codingagent.Requirement{{ID: "requirement-1", Outcome: "Approved behavior works"}}}
		if role == codingagent.Review {
			assignment.Requirements = nil
			assignment.Task = &workload.Tasks[0]
			assignment.ReadFiles = []string{"feature.go"}
		}
		if role == codingagent.OverallReview {
			assignment.Workload = &workload
		}
		callID := appendEntry(codingagent.Entry{Phase: "call_opened", Role: role, TaskID: task, Assignment: assignment})
		appendEntry(codingagent.Entry{Phase: "native_event", Role: role, TaskID: task, CallID: callID})
		content := json.RawMessage(`{"findings":[],"questions":[]}`)
		if role == codingagent.Plan {
			content, _ = json.Marshal(workload)
		}
		appendEntry(codingagent.Entry{Phase: "call_completed", Role: role, TaskID: task, CallID: callID, Result: &codingagent.Result{CallID: callID, Provider: route.Provider, Model: route.Model, Completed: true, ThreadID: "thread-fixture", TurnID: "turn-fixture", Content: content}})
		return callID
	}
	planner := call(codingagent.Plan, "")
	appendEntry(codingagent.Entry{Phase: "plan_frozen", CallID: planner, Workload: &workload})
	reviewer := call(codingagent.Review, "task-1")
	appendEntry(codingagent.Entry{Phase: "task_accepted", TaskID: "task-1", CallID: reviewer, Checks: []codingagent.CheckResult{{ID: "task-check", ExitCode: 0}}})
	overall := call(codingagent.OverallReview, "")
	appendEntry(codingagent.Entry{Phase: "overall_accepted", CallID: overall, Checks: []codingagent.CheckResult{{ID: "overall-check", ExitCode: 0}}})
	return entries
}

func TestCodingAgentCompletionRequiresCurrentIndependentAcceptance(t *testing.T) {
	cases := []struct {
		name      string
		mutate    func([]codingagent.Entry) []codingagent.Entry
		wantError bool
	}{
		{name: "accepted native reviews", mutate: func(entries []codingagent.Entry) []codingagent.Entry { return entries }},
		{name: "missing overall acceptance", wantError: true, mutate: func(entries []codingagent.Entry) []codingagent.Entry { return entries[:len(entries)-1] }},
		{name: "reopened after overall review", wantError: true, mutate: func(entries []codingagent.Entry) []codingagent.Entry {
			return append(entries, codingagent.Entry{ID: 99, Generation: 1, Phase: "task_reopened", TaskID: "task-1"})
		}},
		{name: "unanswered question", wantError: true, mutate: func(entries []codingagent.Entry) []codingagent.Entry {
			return append(entries, codingagent.Entry{ID: 99, Generation: 1, Phase: "questions", Questions: []string{"Choose behavior"}})
		}},
		{name: "wrong generation", wantError: true, mutate: func(entries []codingagent.Entry) []codingagent.Entry {
			entries[len(entries)-1].Generation = 2
			return entries
		}},
		{name: "failed overall check", wantError: true, mutate: func(entries []codingagent.Entry) []codingagent.Entry {
			entries[len(entries)-1].Checks[0].ExitCode = 1
			return entries
		}},
		{name: "invented passing check", wantError: true, mutate: func(entries []codingagent.Entry) []codingagent.Entry {
			entries[len(entries)-1].Checks[0].ID = "unfrozen-check"
			return entries
		}},
		{name: "missing native events", wantError: true, mutate: func(entries []codingagent.Entry) []codingagent.Entry {
			for i := range entries {
				if entries[i].Phase == "native_event" {
					entries[i].Phase = "unrelated"
				}
			}
			return entries
		}},
		{name: "foreign review route", wantError: true, mutate: func(entries []codingagent.Entry) []codingagent.Entry {
			for i := range entries {
				if entries[i].Phase == "call_completed" && entries[i].Role == codingagent.Review {
					entries[i].Result.Model = "other-model"
				}
			}
			return entries
		}},
		{name: "unresolved independent finding", wantError: true, mutate: func(entries []codingagent.Entry) []codingagent.Entry {
			for i := range entries {
				if entries[i].Phase == "call_completed" && entries[i].Role == codingagent.OverallReview {
					entries[i].Result.Content = json.RawMessage(`{"findings":[{"task_id":"task-1","requirement":"Approved behavior works","observation":"Failure"}],"questions":[]}`)
				}
			}
			return entries
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			err := validateCodingAgentCompletionJournal(test.mutate(codingAgentCompletionJournalFixture()), 1, codingAgentTestConfiguration())
			if (err != nil) != test.wantError {
				t.Fatalf("completion error=%v, want error=%v", err, test.wantError)
			}
		})
	}
}
