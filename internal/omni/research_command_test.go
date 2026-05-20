package omni

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/websearch"
)

func TestHandleResearchTurnStoresWebResultsInMemory(t *testing.T) {
	app := NewApp(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	app.web = fakeWebSearchService{
		results: []websearch.Result{{
			Provider:    "duckduckgo",
			SearchURL:   "https://duckduckgo.com/html/?q=tailwind+docs",
			URL:         "https://tailwindcss.com/docs/display",
			Title:       "Display - Tailwind CSS",
			Content:     "Utilities for controlling the display box type of an element.",
			RetrievedAt: time.Date(2026, 5, 18, 18, 0, 0, 0, time.UTC),
		}},
	}
	runner := newFakeMemoryRunner()
	app.memory = NewPGMemoryStore(runner)
	app.runLogger, _ = NewRunLogger(t.TempDir(), "research-command-test")
	defer app.runLogger.Close()

	session := &Session{WorkspacePath: t.TempDir(), WorkspaceHash: "research-command-test", Permission: PermissionFull}
	turn, response, err := app.handleResearchTurn(session, "tailwind display documentation")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(response, "Stored 1 web research memory chunk") {
		t.Fatalf("response = %q", response)
	}
	if turn.ReasonCodes[0] != "web_research_memory" {
		t.Fatalf("reason codes = %#v", turn.ReasonCodes)
	}
	memories, err := app.memory.SearchMemory(context.Background(), "display box type", []string{"web-research"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(memories) != 1 {
		t.Fatalf("memories = %d, want 1", len(memories))
	}
}

func TestHandleResearchTurnReportsMissingMemoryDB(t *testing.T) {
	app := NewApp(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	app.web = fakeWebSearchService{}
	app.runLogger, _ = NewRunLogger(t.TempDir(), "research-no-db-test")
	defer app.runLogger.Close()

	session := &Session{WorkspacePath: t.TempDir(), WorkspaceHash: "research-no-db-test", Permission: PermissionFull}
	turn, response, err := app.handleResearchTurn(session, "weather Thailand")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(response, "Postgres memory is not configured") {
		t.Fatalf("response = %q", response)
	}
	if countEventsOfType(turn.Events, "research_blocked") != 1 {
		t.Fatalf("expected research_blocked event: %#v", turn.Events)
	}
}

func TestResearchCommandQuery(t *testing.T) {
	query, ok := researchCommandQuery("/research Thailand weather now")
	if !ok || query != "Thailand weather now" {
		t.Fatalf("query=%q ok=%t", query, ok)
	}
}

func TestAutoResearchForTurnCapturesContextAndStoresMemory(t *testing.T) {
	app := NewApp(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	app.web = fakeWebSearchService{
		results: []websearch.Result{{
			Provider:    "duckduckgo",
			SearchURL:   "https://duckduckgo.com/html/?q=weather+Thailand+now",
			URL:         "https://example.com/weather",
			Title:       "Thailand weather now",
			Content:     "Bangkok is hot and humid with scattered clouds.",
			RetrievedAt: time.Date(2026, 5, 18, 18, 0, 0, 0, time.UTC),
		}},
	}
	runner := newFakeMemoryRunner()
	app.memory = NewPGMemoryStore(runner)

	plan := ContextToolPlan{
		NeedsWebResearch: true,
		NeedsMemory:      true,
		NeedsShell:       true,
		RequireEvidence:  true,
		Tools:            []string{"web_research", "memory", "shell"},
		Reason:           "test plan",
	}
	events, observation := app.autoResearchForTurn(context.Background(), "what is the weather right now in Thailand?", plan)

	if observation == nil {
		t.Fatal("expected auto research observation")
	}
	if observation.Command != "AUTO_RESEARCH: what is the weather right now in Thailand?" {
		t.Fatalf("command = %q", observation.Command)
	}
	if !strings.Contains(observation.Stdout, "Bangkok is hot and humid") {
		t.Fatalf("observation stdout missing search context:\n%s", observation.Stdout)
	}
	if countEventsOfType(events, "auto_research_completed") != 1 {
		t.Fatalf("missing auto_research_completed: %#v", events)
	}
	if countEventsOfType(events, "auto_research_memory_stored") != 1 {
		t.Fatalf("missing auto_research_memory_stored: %#v", events)
	}
	memories, err := app.memory.SearchMemory(context.Background(), "Bangkok", []string{"web-research"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(memories) != 1 {
		t.Fatalf("memories = %d, want 1", len(memories))
	}
}

func TestHandleTurnUsesStructuredLLMCommandPath(t *testing.T) {
	stdout := &bytes.Buffer{}
	app := NewApp(strings.NewReader(""), stdout, &bytes.Buffer{})
	client, closeServer := fakeOllamaClient(t, []string{
		`{"command":"printf 'structured-chat-result\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"structured chat complete"}`,
	})
	defer closeServer()
	app.ollama = client
	app.runLogger, _ = NewRunLogger(t.TempDir(), "structured-chat-turn-test")
	defer app.runLogger.Close()

	session := &Session{WorkspacePath: t.TempDir(), WorkspaceHash: "structured-chat-turn-test", Permission: PermissionFull}
	turn, response, err := app.handleTurn(session, "what is the weather right now in Thailand?", &activityIndicator{})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(response, "Command: printf 'structured-chat-result\n'") {
		t.Fatalf("response = %q", response)
	}
	if !strings.Contains(response, "Stdout: structured-chat-result") {
		t.Fatalf("response missing stdout = %q", response)
	}
	if !strings.Contains(response, "Answer: structured chat complete") {
		t.Fatalf("response missing answer = %q", response)
	}
	if stdout.String() != "" {
		t.Fatalf("chat command output should be buffered into response, got direct stdout = %q", stdout.String())
	}
	if countEventsOfType(turn.Events, "structured_command_completed") != 1 {
		t.Fatalf("missing structured command event: %#v", turn.Events)
	}
	if countEventsOfType(turn.Events, "final_response_review_passed") != 1 {
		t.Fatalf("missing final response review event: %#v", turn.Events)
	}
}

func TestHandleTurnStoresCapabilityMemoryFromEvaluatorRejection(t *testing.T) {
	app := NewApp(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	client, closeServer := fakeOllamaClient(t, []string{
		`{"command":"printf 'I cannot access real-time information. Check the current time using a time zone app.\n'","done":false,"answer":""}`,
		`{"command":"TZ=America/New_York date '+%Z'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"Virginia time checked."}`,
	})
	defer closeServer()
	app.ollama = client
	app.evaluator = &fakeStructuredResponseEvaluator{evaluations: []StructuredLLMEvaluation{
		{Confidence: 10, Feedback: "False limitation; use command evidence."},
		{Confidence: 95, Feedback: ""},
		{Confidence: 95, Feedback: ""},
	}}
	app.evaluatorThreshold = 70
	app.runLogger, _ = NewRunLogger(t.TempDir(), "structured-memory-test")
	defer app.runLogger.Close()

	session := &Session{WorkspacePath: t.TempDir(), WorkspaceHash: "structured-memory-test", Permission: PermissionFull}
	turn, response, err := app.handleTurn(session, "what time is it in Virginia right now?", &activityIndicator{})
	if err != nil {
		t.Fatal(err)
	}
	if len(session.Memories) != 1 {
		t.Fatalf("session memories = %#v, want one capability memory", session.Memories)
	}
	if !strings.Contains(session.Memories[0].Content, "do not claim no real-time access") {
		t.Fatalf("unexpected memory: %#v", session.Memories[0])
	}
	if countEventsOfType(turn.Events, "capability_memory_stored") != 1 {
		t.Fatalf("missing capability memory event: %#v", turn.Events)
	}
	if strings.Contains(response, "I cannot access real-time") {
		t.Fatalf("rejected response leaked into final response: %q", response)
	}
}

func TestHandleTurnStoresCapabilityMemoryFromDeterministicRejection(t *testing.T) {
	app := NewApp(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	client, closeServer := fakeOllamaClient(t, []string{
		`{"command":"echo 'I do not have access to real-time information. Check the current time using a time zone app.'","done":false,"answer":""}`,
		`{"command":"TZ=America/New_York date '+%Z'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"Virginia time checked."}`,
	})
	defer closeServer()
	app.ollama = client
	app.runLogger, _ = NewRunLogger(t.TempDir(), "structured-validator-memory-test")
	defer app.runLogger.Close()

	session := &Session{WorkspacePath: t.TempDir(), WorkspaceHash: "structured-validator-memory-test", Permission: PermissionFull}
	turn, _, err := app.handleTurn(session, "what time is it in Virginia right now?", &activityIndicator{})
	if err != nil {
		t.Fatal(err)
	}
	if len(session.Memories) != 1 {
		t.Fatalf("session memories = %#v, want deterministic validator capability memory", session.Memories)
	}
	if countEventsOfType(turn.Events, "capability_memory_stored") != 1 {
		t.Fatalf("missing capability memory event: %#v", turn.Events)
	}
}

func TestHandleTurnStoresWeatherCapabilityMemoryFromOpenWeatherMapRejection(t *testing.T) {
	app := NewApp(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	client, closeServer := fakeOllamaClient(t, []string{
		`{"command":"curl -s \"http://api.openweathermap.org/data/2.5/weather?q=Pattaya&appid=YOUR_API_KEY&units=metric\"","done":false,"answer":""}`,
		`{"command":"printf 'Pattaya weather evidence\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"Pattaya weather evidence."}`,
	})
	defer closeServer()
	app.ollama = client
	app.runLogger, _ = NewRunLogger(t.TempDir(), "structured-weather-memory-test")
	defer app.runLogger.Close()

	session := &Session{WorkspacePath: t.TempDir(), WorkspaceHash: "structured-weather-memory-test", Permission: PermissionFull}
	turn, _, err := app.handleTurn(session, "Okay, what is the weather in Pattaya right now?", &activityIndicator{})
	if err != nil {
		t.Fatal(err)
	}
	if len(session.Memories) != 1 {
		t.Fatalf("session memories = %#v, want one weather capability memory", session.Memories)
	}
	if session.Memories[0].Content != structuredWeatherCapabilityMemory {
		t.Fatalf("unexpected weather memory: %#v", session.Memories[0])
	}
	if countEventsOfType(turn.Events, "capability_memory_stored") != 1 {
		t.Fatalf("missing capability memory event: %#v", turn.Events)
	}
}

func TestHandleTurnPassesSessionHistoryToStructuredCommandPath(t *testing.T) {
	app := NewApp(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	requests := []OllamaChatRequest{}
	responses := []string{
		`{"command":"printf 'history says Pattaya\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"Using Pattaya from session history."}`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/create" || r.URL.Path == "/api/delete" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"success"}`))
			return
		}
		var raw struct {
			Messages []OllamaMessage `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			t.Fatal(err)
		}
		requests = append(requests, OllamaChatRequest{Messages: raw.Messages})
		if len(requests) == 1 {
			if len(raw.Messages) < 3 {
				t.Fatalf("structured request missing separated history and active task messages: %#v", raw.Messages)
			}
			historyMessage := raw.Messages[0].Content
			activeMessage := raw.Messages[len(raw.Messages)-1].Content
			if !strings.Contains(historyMessage, "reference_history") || !strings.Contains(historyMessage, "Pattaya") {
				t.Fatalf("structured request missing session history: %s", historyMessage)
			}
			if strings.Contains(activeMessage, "Pattaya") {
				t.Fatalf("active task should not contain copied session history: %s", activeMessage)
			}
		}
		if len(requests) > len(responses) {
			t.Fatalf("unexpected ollama request %d", len(requests))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"model":      "fake",
			"created_at": "2026-05-19T00:00:00Z",
			"done":       true,
			"message": map[string]string{
				"role":    "assistant",
				"content": responses[len(requests)-1],
			},
		})
	}))
	defer server.Close()
	app.ollama = NewOllamaClient(server.URL, "fake")
	app.runLogger, _ = NewRunLogger(t.TempDir(), "structured-history-test")
	defer app.runLogger.Close()

	session := &Session{
		WorkspacePath: t.TempDir(),
		WorkspaceHash: "structured-history-test",
		Permission:    PermissionFull,
		Messages: []Message{
			{Role: "user", Content: "what is the weather in Pattaya Thailand today?"},
			{Role: "assistant", Content: "The weather in Pattaya, Thailand today is Partly Cloudy with temperatures ranging from +31°C to +36°C."},
		},
	}
	_, response, err := app.handleTurn(session, "The weather where will be sunny?", &activityIndicator{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(response, "history says Pattaya") {
		t.Fatalf("response missing history-resolved command output: %q", response)
	}
}

func TestStructuredCommandChatResponseIncludesAPIErrorOutput(t *testing.T) {
	response := formatStructuredCommandChatResponse(
		CommandDecisionResult{
			Command:  "curl -s https://example.test/weather",
			ExitCode: 0,
		},
		`{"cod":401,"message":"Invalid API key"}`,
		"",
		"",
	)

	if !strings.Contains(response, `Stdout: {"cod":401,"message":"Invalid API key"}`) {
		t.Fatalf("response = %q", response)
	}
}

func TestStructuredCommandChatResponseSeparatesPlannerErrorAfterProgress(t *testing.T) {
	response := formatStructuredCommandChatResponse(
		CommandDecisionResult{
			Command:         "npm init -y",
			ExitCode:        0,
			PartialProgress: true,
			ObjectiveLedger: []StructuredObjective{
				{ID: "install_webpack", Status: "pending"},
			},
		},
		"Wrote to package.json",
		"",
		"context deadline exceeded",
	)

	for _, want := range []string{
		"Last command exit code: 0",
		"Pending objectives: install_webpack",
		"Planner error after progress: context deadline exceeded",
	} {
		if !strings.Contains(response, want) {
			t.Fatalf("response missing %q:\n%s", want, response)
		}
	}
	if strings.Contains(response, "Exit code: 1") {
		t.Fatalf("response should not report the successful command as failed:\n%s", response)
	}
}

func TestHandleTurnFinalResponseReviewerCanReviseResponse(t *testing.T) {
	app := NewApp(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	client, closeServer := fakeOllamaClient(t, []string{
		`{"command":"printf 'structured-chat-result\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"structured chat complete"}`,
	})
	defer closeServer()
	app.ollama = client
	app.evaluator = &fakeStructuredResponseEvaluator{evaluations: []StructuredLLMEvaluation{
		{Confidence: 95, Feedback: ""},
		{Confidence: 95, Feedback: ""},
		{Confidence: 20, Feedback: "The final response is off task."},
	}}
	app.evaluatorThreshold = 70
	app.runLogger, _ = NewRunLogger(t.TempDir(), "final-review-revision-test")
	defer app.runLogger.Close()

	session := &Session{WorkspacePath: t.TempDir(), WorkspaceHash: "final-review-revision-test", Permission: PermissionFull}
	turn, response, err := app.handleTurn(session, "summarize the structured result", &activityIndicator{})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(response, "Self-review flagged") {
		t.Fatalf("response was not revised: %q", response)
	}
	if countEventsOfType(turn.Events, "final_response_review_revised") != 1 {
		t.Fatalf("missing final response review revision event: %#v", turn.Events)
	}
}

func TestParseContextToolPlanSelectsSkills(t *testing.T) {
	plan, err := ParseContextToolPlan(`{"tools":["web_research","memory","shell"],"allow_clarify":false,"require_evidence":true,"reason":"external current fact"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.NeedsWebResearch || !plan.NeedsMemory || !plan.NeedsShell {
		t.Fatalf("plan did not select expected tools: %#v", plan)
	}
	if !plan.RequireEvidence {
		t.Fatalf("plan should require evidence: %#v", plan)
	}
	for _, want := range []string{"web_research", "memory", "shell"} {
		if !containsString(plan.Tools, want) {
			t.Fatalf("plan tools missing %q: %#v", want, plan.Tools)
		}
	}

	local, err := ParseContextToolPlan(`{"tools":["shell"],"allow_clarify":false,"require_evidence":false,"reason":"local action"}`)
	if err != nil {
		t.Fatal(err)
	}
	if local.NeedsWebResearch || local.NeedsMemory || !local.NeedsShell {
		t.Fatalf("local plan wrong: %#v", local)
	}
}

func TestPlanContextToolsUsesLLMStructuredDecision(t *testing.T) {
	client, closeServer := fakeOllamaClient(t, []string{
		`{"tools":["web_research","memory","shell"],"allow_clarify":false,"require_evidence":true,"reason":"external current fact"}`,
	})
	defer closeServer()

	plan, err := PlanContextTools(context.Background(), client, "any user text")
	if err != nil {
		t.Fatal(err)
	}
	if !plan.NeedsWebResearch || !plan.NeedsMemory || !plan.NeedsShell || !plan.RequireEvidence {
		t.Fatalf("plan = %#v", plan)
	}
}

func extractTranscriptFromTurnForTest(turn Turn) []CommandObservation {
	out := []CommandObservation{}
	for _, event := range turn.Events {
		if event.Type == "auto_research_completed" {
			out = append(out, CommandObservation{Command: "AUTO_RESEARCH", Status: "success"})
		}
	}
	return out
}
