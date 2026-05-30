package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSplitProjectPathPlanningChat(t *testing.T) {
	id, action := splitProjectPath("/v1/projects/12/planning-chat")
	if id != 12 || action != "planning-chat" {
		t.Fatalf("id=%d action=%q", id, action)
	}
}

func TestNormalizeProjectPlanningMode(t *testing.T) {
	if got := normalizeProjectPlanningMode("/research kubernetes patterns", ""); got != "research" {
		t.Fatalf("mode=%q want research", got)
	}
	if got := normalizeProjectPlanningMode("hello", "plan"); got != "plan" {
		t.Fatalf("mode=%q want plan", got)
	}
}

func TestParseProjectPlanningLLMResponse(t *testing.T) {
	raw := `{"reply":"Hello planner","suggestions":[{"level":"tip","text":"Split the epic"}],"card_drafts":[{"title":"Add auth","column":"backlog"}]}`
	parsed := parseProjectPlanningLLMResponse(raw)
	if parsed.Reply != "Hello planner" {
		t.Fatalf("reply=%q", parsed.Reply)
	}
	if len(parsed.Suggestions) != 1 || len(parsed.CardDrafts) != 1 {
		t.Fatalf("suggestions=%d drafts=%d", len(parsed.Suggestions), len(parsed.CardDrafts))
	}
}

func TestSummarizeScrumBoard(t *testing.T) {
	board := ScrumBoard{
		Columns: []string{"backlog", "ready"},
		Cards: []ScrumCard{
			{Title: "First", Column: "backlog", Description: "Do thing"},
			{Title: "Second", Column: "ready", PlayState: "running"},
		},
	}
	lines := summarizeScrumBoard(board)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "First") || !strings.Contains(joined, "Second") {
		t.Fatalf("summary=%q", joined)
	}
}

func TestLoadProjectPlanningChatFromSettings(t *testing.T) {
	settings := json.RawMessage(`{"planning_chat":[{"role":"user","content":"hi","created_at":"2026-01-01T00:00:00Z"}],"planning_chat_config":{"reasoning_mode":"thinking","model":"qwen3:4b"}}`)
	chat, cfg := loadProjectPlanningChat(settings)
	if len(chat) != 1 || chat[0].Content != "hi" {
		t.Fatalf("chat=%+v", chat)
	}
	if cfg.ReasoningMode != "thinking" || cfg.Model != "qwen3:4b" {
		t.Fatalf("cfg=%+v", cfg)
	}
}

func TestProjectPlanningChatRequiresDatabase(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	req := httptest.NewRequest(http.MethodGet, "/v1/projects/1/planning-chat", nil)
	rec := httptest.NewRecorder()
	server.handleProjectPlanningChat(rec, req, 1)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
