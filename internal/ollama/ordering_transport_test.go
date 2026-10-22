package ollama

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gryph/omnidex/internal/llm"
)

func TestParserLightweightOrderingTransport(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request["prompt"] != "one ordering question" || request["model"] != "fixture-model" {
			t.Errorf("question changed: %+v", request)
		}
		json.NewEncoder(w).Encode(map[string]any{"created_at": "2026-10-20T12:00:00Z", "response": `["b","a"]`, "done": true, "done_reason": "stop", "prompt_eval_count": 40, "eval_count": 12})
	}))
	defer server.Close()
	parser, err := NewParser(server.URL, 32768)
	if err != nil {
		t.Fatal(err)
	}
	prepared := llm.PreparedModel{Protocol: llm.ExactPreparedProtocolPlainCompletionV4, BaseModel: "fixture-model", ContextModel: "fixture-model", Prompt: "one ordering question", MaxOutputTokens: 2048, OutputLimitMode: llm.ExactPreparedOutputLimitExplicit, ContextTokens: 32768}
	result, err := parser.GeneratePreparedExact(t.Context(), prepared)
	if err != nil {
		t.Fatal(err)
	}
	if err := llm.ValidateExactPreparedGenerationForRequest(prepared, result); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || result.Content != `["b","a"]` {
		t.Fatal("unexpected inference/result", calls, result.Content)
	}
}
