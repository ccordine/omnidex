package omni

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestSessionStorePersistsConversationHistory(t *testing.T) {
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
	if reloaded.Messages[0].Content != session.Messages[0].Content || reloaded.Messages[1].Content != session.Messages[1].Content {
		t.Fatalf("persisted messages changed: %#v", reloaded.Messages)
	}
}

func capturingOllamaClient(t *testing.T, responses []string, requests *[]OllamaChatRequest) (*OllamaClient, func()) {
	t.Helper()
	index := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/create" || r.URL.Path == "/api/delete" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success"}`))
			return
		}
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
