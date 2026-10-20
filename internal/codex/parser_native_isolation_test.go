package codex

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Opt-in native contract check: a local endpoint captures only synthetic input,
// never headers or credentials, and no inference reaches an external service.
func TestParserNativeInstructionIsolation(t *testing.T) {
	command := os.Getenv("OMNI_TEST_CODEX_NATIVE")
	if command == "" {
		t.Skip("OMNI_TEST_CODEX_NATIVE is unset")
	}
	payloads := make(chan []byte, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if strings.Contains(r.URL.Path, "responses") {
			select {
			case payloads <- data:
			default:
			}
		}
		http.Error(w, "synthetic endpoint", http.StatusBadRequest)
	}))
	defer server.Close()
	root := t.TempDir()
	t.Setenv("CODEX_HOME", root)
	catalog, err := os.ReadFile(os.Getenv("OMNI_TEST_CODEX_CATALOG"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "models_cache.json"), catalog, 0600); err != nil {
		t.Fatal(err)
	}
	wrapper := filepath.Join(root, "provider")
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
	args := []string{"model_provider=parser_fixture", "model_providers.parser_fixture.name=parser_fixture", "model_providers.parser_fixture.base_url=" + quote(server.URL), "model_providers.parser_fixture.wire_api=responses", "model_providers.parser_fixture.requires_openai_auth=false", "model_providers.parser_fixture.request_max_retries=0"}
	script := "#!/bin/sh\nexec " + quote(command) + " \"$@\""
	for _, arg := range args {
		script += " -c " + quote(arg)
	}
	if err := os.WriteFile(wrapper, []byte(script+"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	client, err := NewParser(context.Background(), wrapper, root, 15*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_, generationErr := client.GeneratePreparedExact(t.Context(), parserRequest(t, client, "synthetic isolation question"))
	select {
	case data := <-payloads:
		var body map[string]json.RawMessage
		if err := json.Unmarshal(data, &body); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"tools", "additional_tools"} {
			if raw := body[key]; len(raw) > 0 && string(raw) != "[]" && string(raw) != "null" {
				t.Fatalf("native provider exposes %s: %s", key, raw)
			}
		}
		var input []struct {
			Type    string `json:"type"`
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(body["input"], &input); err != nil {
			t.Fatal(err)
		}
		expected := []struct{ role, text string }{
			{"developer", "Answer the supplied semantic question using its supplied context. Return only the requested result."},
			{"user", "synthetic isolation question"},
		}
		if len(input) != len(expected) {
			t.Fatalf("native provider injected context: %d items", len(input))
		}
		for i, want := range expected {
			got := input[i]
			if got.Type != "message" || got.Role != want.role || len(got.Content) != 1 || got.Content[0].Type != "input_text" || got.Content[0].Text != want.text {
				t.Fatalf("unexpected native context item %d", i)
			}
		}
		if string(body["model"]) != `"gpt-6.1-sol"` {
			t.Fatal("native model route changed")
		}
		var reasoning struct {
			Effort string `json:"effort"`
		}
		if err := json.Unmarshal(body["reasoning"], &reasoning); err != nil || reasoning.Effort != "low" {
			t.Fatal("native effort changed")
		}
		t.Log("native input: exactly one semantic instruction and one question; no tools; gpt-6.1-sol low")

	case <-time.After(time.Second):
		t.Fatalf("native provider did not reach synthetic endpoint: %v", generationErr)
	}
}
