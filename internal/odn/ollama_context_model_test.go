package odn

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOllamaChatRawUsesEphemeralContextModel(t *testing.T) {
	createdModel := ""
	chatModel := ""
	deletedModel := ""
	createModelfile := ""
	chatMessages := []map[string]interface{}{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		switch r.URL.Path {
		case "/api/create":
			createdModel = payload["name"].(string)
			createModelfile = payload["modelfile"].(string)
			_, _ = w.Write([]byte(`{"status":"success"}`))
		case "/api/chat":
			chatModel = payload["model"].(string)
			rawMessages := payload["messages"].([]interface{})
			for _, raw := range rawMessages {
				chatMessages = append(chatMessages, raw.(map[string]interface{}))
			}
			_, _ = w.Write([]byte(`{"model":"fake","done":true,"message":{"role":"assistant","content":"{\"command\":\"printf ok\",\"done\":false,\"answer\":\"\"}"}}`))
		case "/api/delete":
			deletedModel = payload["name"].(string)
			_, _ = w.Write([]byte(`{"status":"success"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewOllamaClient(server.URL+"/api/chat", "qwen2.5-coder:7b")
	resp, err := client.ChatRaw(context.Background(), OllamaChatRequest{
		ContextSystem: "planner context only",
		Messages: []OllamaMessage{
			{Role: "system", Content: "should not be sent as chat message"},
			{Role: "user", Content: `{"current_prompt":"do the thing"}`},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Content == "" {
		t.Fatal("empty response")
	}
	if createdModel == "" || !strings.HasPrefix(createdModel, "odnctx-") {
		t.Fatalf("created model = %q", createdModel)
	}
	if chatModel != createdModel {
		t.Fatalf("chat model = %q, want context model %q", chatModel, createdModel)
	}
	if deletedModel != createdModel {
		t.Fatalf("deleted model = %q, want %q", deletedModel, createdModel)
	}
	if !strings.Contains(createModelfile, "FROM qwen2.5-coder:7b") || !strings.Contains(createModelfile, "planner context only") {
		t.Fatalf("bad modelfile: %s", createModelfile)
	}
	if len(chatMessages) != 1 || chatMessages[0]["role"] != "user" {
		t.Fatalf("chat messages should contain only isolated user payload: %#v", chatMessages)
	}
}
