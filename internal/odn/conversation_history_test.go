package odn

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestConversationReplyUsesPriorSessionMessagesForRecall(t *testing.T) {
	requests := []OllamaChatRequest{}
	client, closeServer := capturingOllamaClient(t, []string{"We discussed high school students."}, &requests)
	defer closeServer()

	app := NewApp(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	app.ollama = client
	app.runLogger, _ = NewRunLogger(t.TempDir(), "conversation-recall-test")
	defer app.runLogger.Close()

	session := &Session{
		WorkspacePath: t.TempDir(),
		WorkspaceHash: "conversation-recall-test",
		Permission:    PermissionFull,
		Messages: []Message{
			{Role: "user", Content: "For the math app, remember the audience is high school students."},
			{Role: "assistant", Content: "Recorded: the math app audience is high school students."},
		},
	}

	reply, source := app.conversationReply(session, "What audience did we discuss for the math app?")
	if source != "ollama" {
		t.Fatalf("source = %q, want ollama", source)
	}
	if !strings.Contains(reply, "high school students") {
		t.Fatalf("reply did not recall prior context: %q", reply)
	}
	if len(requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(requests))
	}
	assertOllamaMessagesContain(t, requests[0].Messages, "For the math app, remember the audience is high school students.")
	assertOllamaMessagesContain(t, requests[0].Messages, "What audience did we discuss for the math app?")
	assertOllamaMessagesContain(t, requests[0].Messages, "Use conversation history to answer follow-up")
}

func TestSessionStorePersistsConversationHistoryForFutureRecall(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	store := NewSessionStore(filepath.Join(root, "sessions"))
	session, loaded, err := store.LoadOrCreate(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if loaded {
		t.Fatal("expected new session")
	}
	session.Permission = PermissionFull
	session.Messages = append(session.Messages,
		Message{Role: "user", Content: "Remember my preferred frontend style is dense operational dashboards."},
		Message{Role: "assistant", Content: "Stored preference: dense operational dashboards."},
	)
	if err := store.Save(session); err != nil {
		t.Fatal(err)
	}

	reloaded, loaded, err := store.LoadOrCreate(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded {
		t.Fatal("expected persisted session")
	}
	if len(reloaded.Messages) != 2 {
		t.Fatalf("messages = %#v", reloaded.Messages)
	}

	requests := []OllamaChatRequest{}
	client, closeServer := capturingOllamaClient(t, []string{"Your preferred frontend style is dense operational dashboards."}, &requests)
	defer closeServer()
	app := NewApp(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	app.ollama = client
	app.runLogger, _ = NewRunLogger(t.TempDir(), "conversation-persisted-recall-test")
	defer app.runLogger.Close()

	reply, source := app.conversationReply(reloaded, "What frontend style did I say I prefer?")
	if source != "ollama" {
		t.Fatalf("source = %q, want ollama", source)
	}
	if !strings.Contains(reply, "dense operational dashboards") {
		t.Fatalf("reply did not use persisted context: %q", reply)
	}
	assertOllamaMessagesContain(t, requests[0].Messages, "dense operational dashboards")
}

func TestConversationReplyUsesBoundedRecentHistoryWindow(t *testing.T) {
	requests := []OllamaChatRequest{}
	client, closeServer := capturingOllamaClient(t, []string{"Recent context retained."}, &requests)
	defer closeServer()

	app := NewApp(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	app.ollama = client
	app.runLogger, _ = NewRunLogger(t.TempDir(), "conversation-window-test")
	defer app.runLogger.Close()

	session := &Session{WorkspacePath: t.TempDir(), WorkspaceHash: "conversation-window-test", Permission: PermissionFull}
	session.Messages = append(session.Messages, Message{Role: "user", Content: "oldest forgotten marker alpha"})
	for i := 0; i < maxConversationHistoryMessages; i++ {
		session.Messages = append(session.Messages, Message{Role: "user", Content: "recent retained marker beta"})
	}

	_, source := app.conversationReply(session, "What recent marker is retained?")
	if source != "ollama" {
		t.Fatalf("source = %q, want ollama", source)
	}
	if len(requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(requests))
	}
	joined := joinOllamaMessageContent(requests[0].Messages)
	if strings.Contains(joined, "oldest forgotten marker alpha") {
		t.Fatalf("oldest message leaked past bounded history window: %s", joined)
	}
	if !strings.Contains(joined, "recent retained marker beta") {
		t.Fatalf("recent history missing: %s", joined)
	}
}

func capturingOllamaClient(t *testing.T, responses []string, requests *[]OllamaChatRequest) (*OllamaClient, func()) {
	t.Helper()
	index := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if index >= len(responses) {
			t.Fatalf("unexpected ollama request %d", index+1)
		}
		var raw struct {
			Messages []OllamaMessage `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Fatal(err)
		}
		*requests = append(*requests, OllamaChatRequest{Messages: raw.Messages})
		content := responses[index]
		index++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"model":      "fake",
			"created_at": "2026-05-19T00:00:00Z",
			"done":       true,
			"message": map[string]string{
				"role":    "assistant",
				"content": content,
			},
		})
	}))
	return NewOllamaClient(server.URL, "fake"), server.Close
}

func assertOllamaMessagesContain(t *testing.T, messages []OllamaMessage, needle string) {
	t.Helper()
	if !strings.Contains(joinOllamaMessageContent(messages), needle) {
		t.Fatalf("messages missing %q: %#v", needle, messages)
	}
}

func joinOllamaMessageContent(messages []OllamaMessage) string {
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		parts = append(parts, message.Content)
	}
	return strings.Join(parts, "\n")
}
