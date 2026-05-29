package omni

import (
	"strings"
	"testing"
)

func TestLatestStructuredRepairContextFindsMostRecentRejection(t *testing.T) {
	observations := []StructuredCommandObservation{
		{Command: "npm run build", ExitCode: 0, Stdout: "ok"},
		{RejectedCommand: "touch src/App.css", ExitCode: 1, Stderr: "shell specialist command rejected: placeholder-only"},
		{Command: "npm test", ExitCode: 0, Stdout: "ok"},
	}
	ctx := latestStructuredRepairContext(observations)
	if !strings.Contains(ctx.Feedback, "placeholder-only") {
		t.Fatalf("expected most recent rejection in history, got %#v", ctx)
	}
	observations = append(observations, StructuredCommandObservation{
		RejectedResponse: `{"command":"echo plan","done":false,"answer":""}`,
		ExitCode:         1,
		Stderr:           "self-evaluation rejected response: not aligned",
	})
	ctx = latestStructuredRepairContext(observations)
	if !strings.Contains(ctx.Feedback, "self-evaluation rejected") {
		t.Fatalf("repair context = %#v", ctx)
	}
	if ctx.RejectedResponse == "" {
		t.Fatalf("expected rejected response preview, got %#v", ctx)
	}
}

func TestBuildStructuredPlannerRepairFollowUpMessages(t *testing.T) {
	messages := buildStructuredPlannerRepairFollowUpMessages(StructuredRepairContext{
		Feedback:         "command rejected: placeholder-only",
		RejectedCommand:  "touch src/App.css",
		RejectedResponse: `{"command":"touch src/App.css","done":false,"answer":""}`,
		Guidance:         "replace placeholder-only output with substantive source content",
	})
	if len(messages) != 2 {
		t.Fatalf("message count = %d, want 2", len(messages))
	}
	if messages[0].Role != "assistant" || messages[1].Role != "user" {
		t.Fatalf("unexpected roles: %#v", messages)
	}
	if !strings.Contains(messages[1].Content, "repair_feedback") {
		t.Fatalf("repair user message missing feedback: %s", messages[1].Content)
	}
}

func TestBuildStructuredCommandMessagesIncludesRepairFollowUpAfterRejection(t *testing.T) {
	observations := []StructuredCommandObservation{
		{
			RejectedResponse: `{"command":"echo plan","done":false,"answer":""}`,
			ExitCode:         1,
			Stderr:           "command rejected: pure echo command is not command evidence",
		},
	}
	messages := buildStructuredCommandMessagesWithPrep("build app", nil, nil, observations, t.TempDir(), nil, MinimalContext{}, nil, WorksiteSurvey{}, PrepContextBundle{})
	if len(messages) < 3 {
		t.Fatalf("expected repair follow-up messages, got %d", len(messages))
	}
	last := messages[len(messages)-1]
	if last.Role != "user" || !strings.Contains(last.Content, "repair_feedback") {
		t.Fatalf("missing repair follow-up: %#v", last)
	}
	var userMessage string
	for _, message := range messages {
		if message.Role == "user" && strings.Contains(message.Content, "active_task") {
			userMessage = message.Content
			break
		}
	}
	for _, want := range []string{"latest_rejection_feedback", "rejection_repair_guidance", "pure echo command is not command evidence"} {
		if !strings.Contains(userMessage, want) {
			t.Fatalf("planner user message missing %q:\n%s", want, userMessage)
		}
	}
}

func TestCompactStructuredObservationsPinsLatestRejection(t *testing.T) {
	observations := make([]StructuredCommandObservation, 0, 10)
	for i := 0; i < 8; i++ {
		observations = append(observations, StructuredCommandObservation{
			Step:     i + 1,
			Command:  "ls",
			ExitCode: 0,
			Stdout:   "ok",
		})
	}
	observations = append(observations, StructuredCommandObservation{
		Step:            9,
		RejectedCommand: "touch src/App.css",
		ExitCode:        1,
		Stderr:          "command rejected: placeholder-only",
	})
	observations = append(observations, StructuredCommandObservation{
		Step:     10,
		Command:  "npm test",
		ExitCode: 0,
		Stdout:   "ok",
	})
	compact := compactStructuredObservationsForContext(observations, 3, 200)
	found := false
	for _, obs := range compact {
		if strings.Contains(obs.Stderr, "placeholder-only") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("compacted observations dropped latest rejection: %#v", compact)
	}
}
