package ollama

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/codingagent"
)

// Installed-model contract qualification, not an autonomous workload build.
func TestInstalledLocalResponseParser(t *testing.T) {
	model := os.Getenv("OMNI_TEST_OLLAMA_PARSER_MODEL")
	if model == "" {
		t.Skip("OMNI_TEST_OLLAMA_PARSER_MODEL is required")
	}
	parser, err := NewParser(os.Getenv("OMNI_TEST_OLLAMA_PARSER_URL"), 8192)
	if err != nil {
		t.Fatal(err)
	}
	route := codingagent.Route{Provider: "ollama", Model: model}
	for _, test := range []struct{ name, source, kind string }{
		{"done", "Implemented the requested function and ran its tests. All passed; the task is complete.", "done"},
		{"question", "Should the report include archived entries? I need your answer before continuing.", "question"},
		{"permission", "May I download the package needed to run the checks?", "permission"},
		{"error", "Execution is blocked: the configured compiler is unavailable and required checks cannot run.", "error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			call := codingagent.Assignment{Role: codingagent.ExecutionResponse, Route: route, Response: test.source}
			result := liveParserResult(t, parser, call)
			var got struct {
				Kind string `json:"kind"`
			}
			if err := json.Unmarshal(result.Content, &got); err != nil || got.Kind != test.kind {
				t.Fatalf("classification: %s %v", result.Content, err)
			}
		})
	}
	t.Run("review-binding", func(t *testing.T) {
		call := codingagent.Assignment{Role: codingagent.ReviewResponse, Route: route,
			Task:     &codingagent.Task{ID: "saved-task", Acceptance: []string{"Reject invalid dates."}},
			Response: "I tested the required date validation. Invalid dates are accepted, violating the required rejection behavior. This is a concrete defect. Verification used both valid and invalid dates."}
		result := liveParserResult(t, parser, call)
		var got codingagent.ReviewResponseResult
		if err := json.Unmarshal(result.Content, &got); err != nil || got.Kind != codingagent.ExecutionDone || len(got.Findings) != 1 ||
			got.Findings[0].TaskID != call.Task.ID || got.Findings[0].Requirement != call.Task.Acceptance[0] {
			t.Fatalf("review binding: %s %v", result.Content, err)
		}
	})
	for _, test := range []struct{ name, source, kind string }{
		{"review-optional-diagnostic", "I independently verified export creates the required output. That acceptance criterion passes. An unrelated optional documentation check failed. The required review is complete; no defects against the supplied criterion were found.", "done"},
		{"review-required-blocker", "I could not verify whether export creates the required output because the required runtime is unavailable. Required verification is incomplete.", "error"},
	} {
		t.Run(test.name, func(t *testing.T) {
			call := codingagent.Assignment{Role: codingagent.ReviewResponse, Route: route,
				Task: &codingagent.Task{ID: "export", Acceptance: []string{"Export creates the required output."}}, Response: test.source}
			result := liveParserResult(t, parser, call)
			var got codingagent.ReviewResponseResult
			if err := json.Unmarshal(result.Content, &got); err != nil || string(got.Kind) != test.kind || len(got.Findings) != 0 {
				t.Fatalf("review boundary: %s %v", result.Content, err)
			}
		})
	}
	for _, test := range []struct {
		name, source string
		argv         []string
	}{
		{"initial-planning", "One proposed implementation task: create a Python word-count command. Requirement: accept text and print its word count. Acceptance criterion: empty input prints zero and two words print two. Verify with automated tests covering those cases using `python3 -m unittest discover`. Delivery is local-only.", []string{"python3", "-m", "unittest", "discover"}},
		{"initial-observation", "One proposed task: document a backup restoration procedure in RESTORE.md. Requirement: describe how to restore a saved archive. Acceptance criterion: the document includes prerequisites and restoration steps. Verify by reading the document and confirming those sections exist. Delivery is local-only.", nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			turn := codingagent.PlanningTurn{OperationID: "qualification", OwnerTurnID: "owner", Text: test.source}
			journal := &livePlanningJournal{entries: []codingagent.Entry{{Phase: "planning_response", Notes: []string{turn.OperationID, turn.Text, test.source}}}}
			native := codingagent.Route{Provider: "codex", Model: "not-invoked"}
			controller := codingagent.Controller{JobID: 1, Generation: 1, Root: ".",
				Configuration: codingagent.Configuration{Mode: codingagent.Agents, Planner: native, LedgerParser: route, Executor: native, Reviewer: native},
				Providers:     map[string]codingagent.Provider{"codex": parser, "ollama": parser}, Environment: journal, Evidence: journal,
				Requirements: []codingagent.Requirement{{ID: "objective", Outcome: test.source}}}
			proposal, err := controller.ProposePlanningLedger(context.Background(), turn)
			for _, entry := range journal.entries {
				if entry.Result != nil {
					t.Logf("role=%s output=%s", entry.Role, entry.Result.Content)
				}
			}
			if err != nil {
				t.Fatalf("production planning validation: %v", err)
			}
			if len(proposal.Ledger.Tasks) != 1 || len(proposal.Ledger.Requirements) < 1 || len(proposal.Ledger.Checks) < 1 {
				t.Fatalf("initial ledger lost planned items: %#v", proposal.Ledger)
			}
			for _, check := range proposal.Ledger.Checks {
				if len(test.argv) == 0 && len(check.Argv) != 0 || len(test.argv) > 0 && !reflect.DeepEqual(check.Argv, test.argv) {
					t.Fatalf("parser invented or changed a verification command: %q", check.Argv)
				}
			}
		})
	}
	t.Run("planning-mutation", func(t *testing.T) {
		call := codingagent.Assignment{Role: codingagent.PlanningMutation, Route: route,
			PlanningResponse: "The existing task-1 goal is unchanged. No ledger changes are proposed."}
		result := liveParserResult(t, parser, call)
		var got struct {
			Operations []json.RawMessage `json:"operations"`
		}
		if err := json.Unmarshal(result.Content, &got); err != nil || got.Operations == nil || len(got.Operations) != 0 {
			t.Fatalf("unchanged planning: %s %v", result.Content, err)
		}
	})
}

// Replay a retained native planning reply through the production controller.
// Only the local parser runs; Parser itself rejects any accidental native call.
type livePlanningJournal struct{ entries []codingagent.Entry }

func (j *livePlanningJournal) Append(_ context.Context, entry codingagent.Entry) (int64, error) {
	entry.ID = int64(len(j.entries) + 1)
	j.entries = append(j.entries, entry)
	return entry.ID, nil
}
func (j *livePlanningJournal) Read(context.Context) ([]codingagent.Entry, error) {
	return j.entries, nil
}
func (*livePlanningJournal) ReadTaskClarifications(context.Context, string) ([]codingagent.TaskClarification, error) {
	return nil, nil
}
func (*livePlanningJournal) Attest(context.Context) error { return nil }

func liveParserResult(t *testing.T, parser *Parser, call codingagent.Assignment) codingagent.Result {
	t.Helper()
	started := time.Now()
	result, err := parser.Run(context.Background(), call, func(event codingagent.NativeEvent) error {
		var frame struct {
			Phase   string `json:"phase"`
			Payload struct {
				PromptTokens int `json:"prompt_eval_count"`
				OutputTokens int `json:"eval_count"`
			} `json:"payload"`
		}
		_ = json.Unmarshal(event.Frame, &frame)
		if frame.Phase == "response" {
			t.Logf("model=%s prompt_tokens=%d output_tokens=%d", call.Route.Model, frame.Payload.PromptTokens, frame.Payload.OutputTokens)
		}
		return nil
	})
	if err != nil || result.ValidateFor(call.Role, call.Route) != nil {
		t.Fatalf("parser result: %s %v", result.Content, err)
	}
	t.Logf("role=%s elapsed=%s output=%s", call.Role, time.Since(started).Round(time.Millisecond), result.Content)
	return result
}
