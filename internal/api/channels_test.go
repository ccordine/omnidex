package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChannelRouteCreatesMemoryBackedNonAgentChannel(t *testing.T) {
	llmClient := &fakeLLMClient{outputs: []string{"I remember the billing preference."}}
	server := NewServer(nil, llmClient)

	createBody := `{
		"id":"support-user-123",
		"name":"Support User 123",
		"persona":"assistant",
		"system":"You are a concise support assistant.",
		"llm":{"model":"support-model"},
		"context":{"product":"Omnidex"},
		"tags":["support","billing"]
	}`
	createReq := httptest.NewRequest(http.MethodPost, "/v1/channels", strings.NewReader(createBody))
	createRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createRec.Code, createRec.Body.String())
	}

	store := server.channelStore.(*inMemoryChannelStore)
	_, err := store.AddMemoryChunk(createReq.Context(), "seed", "preference", "User prefers invoices by email.", []string{"channel:support-user-123", "support"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	messageBody := `{"prompt":"What billing preference should I use?"}`
	msgReq := httptest.NewRequest(http.MethodPost, "/v1/channels/support-user-123/messages", strings.NewReader(messageBody))
	msgRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(msgRec, msgReq)
	if msgRec.Code != http.StatusOK {
		t.Fatalf("message status=%d body=%s", msgRec.Code, msgRec.Body.String())
	}

	var payload channelMessageResponse
	if err := json.Unmarshal(msgRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Output != "I remember the billing preference." {
		t.Fatalf("output=%q", payload.Output)
	}
	if payload.Model != "support-model" {
		t.Fatalf("model=%q", payload.Model)
	}
	if len(payload.Memory) == 0 || !strings.Contains(payload.Memory[0].Content, "invoices by email") {
		t.Fatalf("memory not returned: %#v", payload.Memory)
	}
	if len(llmClient.preparePrompts) != 1 {
		t.Fatalf("prepare calls=%d", len(llmClient.preparePrompts))
	}
	compiled := llmClient.preparePrompts[0]
	for _, want := range []string{"CHANNEL_MEMORY", "invoices by email", "REQUEST_CONTEXT_JSON", "Omnidex", "concise support assistant"} {
		if !strings.Contains(compiled, want) {
			t.Fatalf("compiled prompt missing %q:\n%s", want, compiled)
		}
	}
	messages, err := store.ListChannelMessages(msgReq.Context(), "support-user-123", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 || messages[0].Role != "user" || messages[1].Role != "assistant" {
		t.Fatalf("messages=%#v", messages)
	}
	if len(store.memories) < 3 {
		t.Fatalf("expected seed + persisted user/assistant memories, got %#v", store.memories)
	}
}

func TestChannelRouteSupportsRoleplayPersonaAndRecentHistory(t *testing.T) {
	llmClient := &fakeLLMClient{outputs: []string{"The captain lowers her voice."}}
	server := NewServer(nil, llmClient)

	createBody := `{
		"id":"rp-table",
		"persona":"roleplay",
		"system":"Stay in character as the ship captain.",
		"model":"rp-model"
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/channels", strings.NewReader(createBody))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}
	store := server.channelStore.(*inMemoryChannelStore)
	if _, err := store.AddChannelMessage(req.Context(), "rp-table", "user", "We entered the fog."); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddChannelMessage(req.Context(), "rp-table", "assistant", "Hold the lantern high."); err != nil {
		t.Fatal(err)
	}

	msgReq := httptest.NewRequest(http.MethodPost, "/v1/channels/rp-table/messages", strings.NewReader(`{"prompt":"What do you say next?","remember":false}`))
	msgRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(msgRec, msgReq)
	if msgRec.Code != http.StatusOK {
		t.Fatalf("message status=%d body=%s", msgRec.Code, msgRec.Body.String())
	}
	compiled := llmClient.preparePrompts[0]
	for _, want := range []string{"Write an in-character response", "We entered the fog.", "Hold the lantern high.", "ship captain"} {
		if !strings.Contains(compiled, want) {
			t.Fatalf("compiled prompt missing %q:\n%s", want, compiled)
		}
	}
	if len(store.memories) != 0 {
		t.Fatalf("remember=false should not persist memories: %#v", store.memories)
	}
}
