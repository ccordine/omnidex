package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSplitProjectPathPlanningChat(t *testing.T) {
	id, action := splitProjectPath("/v1/projects/12/planning-chat")
	if id != 12 || action != "planning-chat" {
		t.Fatalf("id=%d action=%q", id, action)
	}
}

func TestNormalizeProjectPlanningModeUsesExplicitTransport(t *testing.T) {
	mode, err := normalizeProjectPlanningMode("")
	if err != nil || mode != "chat" {
		t.Fatalf("mode=%q err=%v want chat", mode, err)
	}
	mode, err = normalizeProjectPlanningMode("plan")
	if err != nil || mode != "plan" {
		t.Fatalf("mode=%q err=%v want plan", mode, err)
	}
	for _, invalid := range []string{"legacy", "config"} {
		if _, err := normalizeProjectPlanningMode(invalid); err == nil {
			t.Fatalf("mode=%q must fail", invalid)
		}
	}
}

func TestProjectPlanningHasNoMessagePrefixRouter(t *testing.T) {
	source := readAPISource(t, "project_planning_config.go") + readAPISource(t, "project_planning_context.go")
	for _, forbidden := range []string{"/researching", `case "/plan":`, `case "/batch":`, "strings.HasPrefix(strings.ToLower(query), prefix)"} {
		if strings.Contains(source, forbidden) {
			t.Errorf("project planning contains forbidden message prefix router %q", forbidden)
		}
	}
}

func TestParseProjectPlanningLLMResponseStrict(t *testing.T) {
	raw := `{"reply":"Hello planner","suggestions":[{"level":"tip","text":"Split the epic"}],"card_drafts":[{"title":"Add auth","description":"Use sessions","column":"backlog","checklist":["Add tests"]}]}`
	parsed, err := parseProjectPlanningLLMResponse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Reply != "Hello planner" || len(parsed.Suggestions) != 1 || len(parsed.CardDrafts) != 1 {
		t.Fatalf("unexpected response: %+v", parsed)
	}
	for _, invalid := range []string{
		`plain text`,
		`{"reply":"ok","unknown":true}`,
		`{"reply":"","suggestions":[],"card_drafts":[]}`,
		`{"reply":"ok","suggestions":[{"level":"maybe","text":"x"}]}`,
		`{"reply":"ok","card_drafts":[{"title":"x","column":"invented"}]}`,
		`{"reply":"ok","card_drafts":[{"title":"x","column":"backlog","checklist":[""]}]}`,
	} {
		if _, err := parseProjectPlanningLLMResponse(invalid); err == nil {
			t.Fatalf("response %s must fail loudly", invalid)
		}
	}
}

func TestValidateProjectPlanningChatConfig(t *testing.T) {
	config, err := validateProjectPlanningChatConfig(ProjectPlanningChatConfig{Model: " qwen ", ReasoningMode: "Thinking"})
	if err != nil || config.Model != "qwen" || config.ReasoningMode != "thinking" {
		t.Fatalf("config=%+v err=%v", config, err)
	}
	if _, err := validateProjectPlanningChatConfig(ProjectPlanningChatConfig{ReasoningMode: "automatic"}); err == nil {
		t.Fatal("unknown reasoning mode must fail")
	}
}

func TestParseProjectPlanningPage(t *testing.T) {
	r := &http.Request{URL: &url.URL{RawQuery: "limit=25&before_id=42"}}
	limit, beforeID, err := parseProjectPlanningPage(r)
	if err != nil || limit != 25 || beforeID != 42 {
		t.Fatalf("limit=%d before=%d err=%v", limit, beforeID, err)
	}
	for _, query := range []string{"limit=101", "limit=nope", "before_id=0", "before_id=nope"} {
		if _, _, err := parseProjectPlanningPage(&http.Request{URL: &url.URL{RawQuery: query}}); err == nil {
			t.Fatalf("query %q must fail", query)
		}
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
	joined := strings.Join(summarizeScrumBoard(board), "\n")
	if !strings.Contains(joined, "First") || !strings.Contains(joined, "Second") {
		t.Fatalf("summary=%q", joined)
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

func TestProjectPlanningUpdatePublishesTypedRealtimeEvent(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	subscription, err := server.realtimeHub.Subscribe([]string{realtimeTopicScrum}, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Unsubscribe()
	published, message := server.publishProjectPlanningUpdated(42, "response_committed")
	if !published || message != "" {
		t.Fatalf("published=%t message=%q", published, message)
	}
	select {
	case raw := <-subscription.Messages:
		var event realtimeMessage
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatal(err)
		}
		if event.EventName != "project-planning-updated" || event.ProjectID != 42 || event.Reason != "response_committed" {
			t.Fatalf("event=%+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for project planning realtime event")
	}
}

func TestProjectPlanningUpdateReportsRealtimeDegradation(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	server.realtimeHub = nil
	published, message := server.publishProjectPlanningUpdated(42, "response_committed")
	if published || !strings.Contains(message, "not initialized") {
		t.Fatalf("published=%t message=%q", published, message)
	}
}
