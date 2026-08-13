package main

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/agentstream"
	"github.com/gryph/omnidex/internal/model"
)

func cliAgentEventLine(t *testing.T, event agentstream.Event) string {
	t.Helper()
	line, err := agentstream.EncodeLine(event)
	if err != nil {
		t.Fatal(err)
	}
	return line
}

func TestFormatExternalAgentCLIEventLineSummarizesCommand(t *testing.T) {
	line := cliAgentEventLine(t, agentstream.Event{SessionID: "s", Agent: "codex", Type: agentstream.EventCommand, Message: "running", Command: "go test ./..."})
	got := formatExternalAgentCLIEventLine(line, 120)
	if !strings.Contains(got, "codex command: go test ./...") {
		t.Fatalf("unexpected summary: %q", got)
	}
}

func TestFormatExternalAgentCLIEventLineReportsMalformedJSONObjects(t *testing.T) {
	if got := formatExternalAgentCLIEventLine(`{"agent":`, 120); !strings.Contains(got, "stream rejected") {
		t.Fatalf("malformed typed event must fail visibly, got %q", got)
	}
}

func TestFormatExternalAgentCLIEventLineKeepsJSONLookingAssistantContentOpaque(t *testing.T) {
	line := cliAgentEventLine(t, agentstream.Event{
		SessionID: "s", Agent: "codex", Type: agentstream.EventMessage,
		Message: `{"type":"error","tool":"delete"}`,
	})
	got := formatExternalAgentCLIEventLine(line, 200)
	if !strings.HasPrefix(got, "codex: ") || !strings.Contains(got, `"type":"error"`) {
		t.Fatalf("assistant content was reclassified: %q", got)
	}
}

func TestPrintExternalAgentStreamUpdatesAdvancesOffset(t *testing.T) {
	step := model.Step{
		ID:     10,
		Action: "external_agent_execute",
		Output: strings.Join([]string{
			cliAgentEventLine(t, agentstream.Event{SessionID: "s", Agent: "cursor", Type: agentstream.EventThinking, Message: "inspecting files"}),
			cliAgentEventLine(t, agentstream.Event{SessionID: "s", Agent: "cursor", Type: agentstream.EventFileChange, Message: "completed", Files: []string{"main.go"}}),
		}, "\n") + "\n",
	}
	offsets := map[int64]int{}
	if !printExternalAgentStreamUpdatesWithUI([]model.Step{step}, offsets, nil, 120) {
		t.Fatal("expected first stream pass to print")
	}
	if offsets[10] != len(step.Output) {
		t.Fatalf("offset=%d want %d", offsets[10], len(step.Output))
	}
	if printExternalAgentStreamUpdatesWithUI([]model.Step{step}, offsets, nil, 120) {
		t.Fatal("expected second identical stream pass to be quiet")
	}
}
