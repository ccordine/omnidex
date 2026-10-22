package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func parserAssignment() codingagent.Assignment {
	return codingagent.Assignment{Role: codingagent.ExecutionResponse,
		Route: codingagent.Route{Provider: "ollama", Model: "parser:7b"}, Response: "The task is finished.",
		Task: &codingagent.Task{ID: "task", Goal: "Finish the task."}}
}

const parserScanFixture = `{"kind":"done"}`

func TestLocalParserUsesSharedSchemaAndOriginalWireEvidence(t *testing.T) {
	dispatches := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dispatches++
		if r.URL.Path != "/api/generate" || r.Method != http.MethodPost {
			t.Errorf("unexpected parser operation: %s %s", r.Method, r.URL.Path)
		}
		var request parserRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Raw || request.Stream || request.Shift || request.Truncate || request.Options.Temperature != 0 ||
			request.Options.ContextTokens != 8192 || request.Options.OutputTokens != 4096 || request.Model != "parser:7b" ||
			!strings.HasSuffix(request.Prompt, parserAssignment().Response) || request.Format["type"] != "object" {
			t.Errorf("parser request changed its bounds or exact source: %+v", request)
		}
		_ = json.NewEncoder(w).Encode(parserResponse{Model: "parser:7b", Done: true, DoneReason: "stop",
			Response: parserScanFixture, PromptEvalCount: 120, EvalCount: 7})
	}))
	defer server.Close()
	parser, err := NewParser(server.URL, 8192)
	if err != nil {
		t.Fatal(err)
	}
	frames := []json.RawMessage{}
	result, err := parser.Run(context.Background(), parserAssignment(), func(event codingagent.NativeEvent) error {
		frames = append(frames, event.Frame)
		return nil
	})
	if err != nil || dispatches != 1 || len(frames) != 2 || !result.Completed || string(result.Content) != parserScanFixture ||
		result.ThreadID != "" || result.TurnID != "" || result.ValidateFor(codingagent.ExecutionResponse, parserAssignment().Route) != nil {
		t.Fatalf("local parser lost evidence or fabricated a native identity: %+v frames=%d err=%v", result, len(frames), err)
	}
	for _, role := range []codingagent.Role{codingagent.Plan, codingagent.Execute, codingagent.Review} {
		call := parserAssignment()
		call.Role = role
		if _, err := parser.Run(context.Background(), call, func(codingagent.NativeEvent) error { return nil }); err == nil {
			t.Fatalf("local parser acquired native %s capability", role)
		}
	}
	if dispatches != 1 {
		t.Fatal("native role reached local inference")
	}
	if result.ValidateFor(codingagent.Execute, parserAssignment().Route) == nil {
		t.Fatal("a parser result was usable as native execution evidence")
	}
}

func TestLocalParserRejectsIncompleteOrUnboundProviderResponses(t *testing.T) {
	for _, name := range []string{"different-model", "length-stop", "unfinished", "malformed-output", "unavailable"} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				response := parserResponse{Model: "parser:7b", Done: true, DoneReason: "stop", Response: parserScanFixture, PromptEvalCount: 100, EvalCount: 8}
				switch name {
				case "different-model":
					response.Model = "other:7b"
				case "length-stop":
					response.DoneReason = "length"
				case "unfinished":
					response.Done = false
				case "malformed-output":
					response.Response = "finished"
				case "unavailable":
					w.WriteHeader(http.StatusServiceUnavailable)
				}
				_ = json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()
			parser, _ := NewParser(server.URL, 8192)
			call := parserAssignment()
			result, err := parser.Run(context.Background(), call, func(codingagent.NativeEvent) error { return nil })
			if err == nil || result.Completed {
				t.Fatalf("%s became an accepted parser result: %+v %v", name, result, err)
			}
		})
	}
}

func TestLocalPlanningExtractsCompleteTaskWithOneRequest(t *testing.T) {
	source := "1. Render the requested page. Use the configured theme.\n\nFiles: page.go and page_test.go.\n\nAcceptance: keyboard navigation works. Verify with the keyboard test."
	dispatches := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dispatches++
		var request parserRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(request.Prompt, source) || strings.Contains(request.Prompt, "group_start") || strings.Contains(request.Prompt, "task_support") {
			t.Fatal("task source was split into parser stages", request.Prompt)
		}
		content, _ := json.Marshal(map[string]any{"ledger": map[string]any{"tasks": []any{map[string]any{
			"goal": source, "objective": "Render the requested page.",
			"owner_context":         []any{map[string]any{"text": "Use the configured theme."}},
			"constraints":           []string{"Preserve keyboard navigation."},
			"write_files":           []string{"page.go", "page_test.go"},
			"file_responsibilities": []any{map[string]any{"path": "page.go", "action": "change", "responsibility": "Render the page."}},
			"acceptance":            []any{map[string]any{"text": "Keyboard navigation works.", "verification": []string{"Run the keyboard test."}}},
		}}}})
		_ = json.NewEncoder(w).Encode(parserResponse{Model: "parser:7b", Done: true, DoneReason: "stop", Response: string(content), PromptEvalCount: 120, EvalCount: 100})
	}))
	defer server.Close()
	parser, err := NewParser(server.URL, 8192)
	if err != nil {
		t.Fatal(err)
	}
	call := codingagent.Assignment{JobID: 1, Role: codingagent.PlanningInitial, PlanningResponse: source, Route: codingagent.Route{Provider: "ollama", Model: "parser:7b"}}
	result, err := parser.Run(t.Context(), call, func(codingagent.NativeEvent) error { return nil })
	if err != nil || dispatches != 1 {
		t.Fatalf("planning required extra stages: requests=%d err=%v", dispatches, err)
	}
	var parsed struct {
		Ledger codingagent.EngineeringLedger `json:"ledger"`
	}
	if err := json.Unmarshal(result.Content, &parsed); err != nil {
		t.Fatal(err)
	}
	if err := parsed.Ledger.ValidateDraft(); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Ledger.Tasks) != 1 || parsed.Ledger.Tasks[0].Goal != source || len(parsed.Ledger.Tasks[0].Acceptance[0].Verification) != 1 || parsed.Ledger.Tasks[0].OwnerContext[0].Provenance.Reference == "" {
		t.Fatalf("task details were lost: %+v", parsed.Ledger)
	}
}
