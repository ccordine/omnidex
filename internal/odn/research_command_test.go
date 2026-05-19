package odn

import (
	"bytes"
	"context"
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
