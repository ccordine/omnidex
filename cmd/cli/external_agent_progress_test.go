package main

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestFormatExternalAgentCLIEventLineSummarizesCommand(t *testing.T) {
	line := `{"agent":"codex","type":"command","message":"running","command":"go test ./..."}`
	got := formatExternalAgentCLIEventLine(line, 120)
	if !strings.Contains(got, "codex command: go test ./...") {
		t.Fatalf("unexpected summary: %q", got)
	}
}

func TestFormatExternalAgentCLIEventLineDropsMalformedJSONObjects(t *testing.T) {
	if got := formatExternalAgentCLIEventLine(`{"agent":`, 120); got != "" {
		t.Fatalf("malformed JSON object should be hidden, got %q", got)
	}
}

func TestPrintExternalAgentStreamUpdatesAdvancesOffset(t *testing.T) {
	step := model.Step{
		ID:     10,
		Action: "external_agent_execute",
		Output: strings.Join([]string{
			`{"agent":"cursor","type":"thinking","message":"inspecting files"}`,
			`{"agent":"cursor","type":"file_change","files":["main.go"]}`,
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
