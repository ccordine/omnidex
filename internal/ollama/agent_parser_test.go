package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/codingagent"
)

func parserAssignment() codingagent.Assignment {
	return codingagent.Assignment{Role: codingagent.ExecutionResponse,
		Route: codingagent.Route{Provider: "ollama", Model: "parser:7b"}, Response: "The task is finished."}
}

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
			Response: `{"kind":"done"}`, PromptEvalCount: 120, EvalCount: 7})
	}))
	defer server.Close()
	parser, err := NewParser(server.URL, time.Second, 8192)
	if err != nil {
		t.Fatal(err)
	}
	frames := []json.RawMessage{}
	result, err := parser.Run(context.Background(), parserAssignment(), func(event codingagent.NativeEvent) error {
		frames = append(frames, event.Frame)
		return nil
	})
	if err != nil || dispatches != 1 || len(frames) != 2 || !result.Completed || string(result.Content) != `{"kind":"done"}` ||
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
	for _, name := range []string{"different-model", "length-stop", "unfinished", "oversized-context", "malformed-output", "invalid-review-reference", "unavailable"} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				response := parserResponse{Model: "parser:7b", Done: true, DoneReason: "stop", Response: `{"kind":"done"}`, PromptEvalCount: 100, EvalCount: 8}
				switch name {
				case "different-model":
					response.Model = "other:7b"
				case "length-stop":
					response.DoneReason = "length"
				case "unfinished":
					response.Done = false
				case "oversized-context":
					response.PromptEvalCount = 8192
				case "malformed-output":
					response.Response = "finished"
				case "invalid-review-reference":
					response.Response = `{"kind":"done","findings":[{"requirement":"criterion_2","file":"","observation":"Broken"}],"verification":"Checked"}`
				case "unavailable":
					w.WriteHeader(http.StatusServiceUnavailable)
				}
				_ = json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()
			parser, _ := NewParser(server.URL, time.Second, 8192)
			call := parserAssignment()
			if name == "invalid-review-reference" {
				call.Role, call.Task = codingagent.ReviewResponse, &codingagent.Task{ID: "task", Acceptance: []string{"Works"}}
			}
			result, err := parser.Run(context.Background(), call, func(codingagent.NativeEvent) error { return nil })
			if err == nil || result.Completed {
				t.Fatalf("%s became an accepted parser result: %+v %v", name, result, err)
			}
		})
	}
}
