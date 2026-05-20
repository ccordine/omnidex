package odn

import (
	"strings"
	"testing"
)

func TestGuardFinalResponsePreservesBlockedEvidence(t *testing.T) {
	result := AgentCommandLoopResult{
		Summary:       "Directory created successfully.",
		BlockedCount:  1,
		ExecutedCount: 1,
		Done:          true,
		Transcript: []CommandObservation{
			{Step: 1, Command: "mkdir -p /home/gryph/Projects/temp-test", Status: "success"},
			{Step: 1, Command: "cd /home/gryph/Projects/temp-test", Status: "blocked", Error: "root_command_not_allowlisted: command \"cd\" is not allowlisted"},
			{Step: 2, Command: "mkdir -p /home/gryph/Projects/temp-test", Status: "success"},
		},
	}

	got := guardFinalResponse("Directory created successfully. No further actions needed.", result)

	if !strings.Contains(got, "Blocked: cd /home/gryph/Projects/temp-test") {
		t.Fatalf("guarded response missing blocked command:\n%s", got)
	}
	if !strings.Contains(got, "Directory created") {
		t.Fatalf("guarded response lost final response:\n%s", got)
	}
	if !strings.Contains(got, "Tried:") || !strings.Contains(got, "Did not work:") || !strings.Contains(got, "Worked:") || !strings.Contains(got, "Final:") {
		t.Fatalf("guarded response missing recap shape:\n%s", got)
	}
}

func TestGuardFinalResponseCorrectsDeniedBlockedEvidence(t *testing.T) {
	result := AgentCommandLoopResult{
		Summary:      "done",
		BlockedCount: 1,
		Done:         true,
		Transcript: []CommandObservation{
			{Step: 1, Status: "blocked", Error: "ASK rejected: objective appears answerable"},
			{Step: 2, Command: "env TZ=America/New_York date", Status: "success", Stdout: "Mon May 18 13:59:24 EDT 2026"},
		},
	}

	got := guardFinalResponse("Current time: Mon May 18 13:59:24 EDT 2026. No blocked or failed items.", result)

	if !strings.Contains(got, "Blocked: ASK rejected") {
		t.Fatalf("guarded response missing blocked correction:\n%s", got)
	}
}

func TestGuardFinalResponseRejectsSpecificFactWithoutOutput(t *testing.T) {
	result := AgentCommandLoopResult{
		Summary:       "2023-04-15T12:34:56",
		ExecutedCount: 1,
		Done:          true,
		Transcript: []CommandObservation{
			{Step: 1, Command: "curl -s http://example.invalid", Status: "success"},
		},
	}

	got := guardFinalResponse("2023-04-15T12:34:56", result)

	if !strings.Contains(got, "No command output was captured") {
		t.Fatalf("guarded response should reject unsupported timestamp:\n%s", got)
	}
}

func TestGuardFinalResponseAllowsFactWithOutput(t *testing.T) {
	result := AgentCommandLoopResult{
		Summary:       "2026-05-18T13:44:06-04:00",
		ExecutedCount: 1,
		Done:          true,
		Transcript: []CommandObservation{
			{Step: 1, Command: "date", Status: "success", Stdout: "Mon May 18 13:44:06 EDT 2026"},
		},
	}

	got := guardFinalResponse("2026-05-18T13:44:06-04:00", result)

	if !strings.Contains(got, "2026-05-18T13:44:06-04:00") {
		t.Fatalf("guarded response = %q", got)
	}
	if !strings.Contains(got, "Worked: date -> Mon May 18 13:44:06 EDT 2026") {
		t.Fatalf("guarded response missing worked evidence:\n%s", got)
	}
}

func TestGuardFinalResponseDoesNotPhraseMatchEagerPhrasing(t *testing.T) {
	result := AgentCommandLoopResult{
		Summary:       "done",
		ExecutedCount: 1,
		Done:          true,
		Transcript: []CommandObservation{
			{Step: 1, Command: "pwd", Status: "success", Stdout: "/tmp/work"},
		},
	}

	got := guardFinalResponse("Successfully completed. Happy to help. No further actions needed.", result)

	if !strings.Contains(got, "Successfully completed. Happy to help. No further actions needed.") {
		t.Fatalf("guarded response should not phrase-match cleanup text:\n%s", got)
	}
	if !strings.Contains(got, "Tried: pwd -> /tmp/work") || !strings.Contains(got, "Worked: pwd -> /tmp/work") {
		t.Fatalf("guarded response missing audit recap:\n%s", got)
	}
}

func TestBuildFinalResponderMessagesIncludesEvidenceOnlyRule(t *testing.T) {
	messages := BuildFinalResponderMessages("/tmp/work", "run pwd", AgentCommandLoopResult{Summary: "done"})

	if len(messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(messages))
	}
	if !strings.Contains(messages[0].Content, "Use only provided execution facts") {
		t.Fatalf("system prompt missing evidence rule:\n%s", messages[0].Content)
	}
	if !strings.Contains(messages[0].Content, "No success gloss") {
		t.Fatalf("system prompt missing anti-appeasement rule:\n%s", messages[0].Content)
	}
	if !strings.Contains(messages[0].Content, "Recap: tried; did not work; worked; final") {
		t.Fatalf("system prompt missing recap rule:\n%s", messages[0].Content)
	}
	if !strings.Contains(messages[1].Content, "Transcript:") {
		t.Fatalf("user prompt missing transcript:\n%s", messages[1].Content)
	}
}

func TestReviewFinalAssistantResponseRejectsEmptyResponse(t *testing.T) {
	review := ReviewFinalAssistantResponse(FinalAssistantResponseReviewInput{
		UserInput: "create a file",
		Response:  "  ",
	})

	if review.Passed {
		t.Fatalf("empty response should not pass: %#v", review)
	}
	if !strings.Contains(review.Response, "could not produce") {
		t.Fatalf("unexpected correction: %q", review.Response)
	}
}

func TestReviewFinalAssistantResponseFlagsOffTaskLongResponse(t *testing.T) {
	review := ReviewFinalAssistantResponse(FinalAssistantResponseReviewInput{
		UserInput: "create a Go CLI demo in this workspace",
		Response:  strings.Repeat("The capital city discussion covers history and restaurants. ", 8),
	})

	if review.Passed {
		t.Fatalf("off-task response should not pass: %#v", review)
	}
	if !strings.Contains(review.Response, "Self-review flagged") {
		t.Fatalf("correction missing self-review context: %q", review.Response)
	}
}

func TestReviewFinalAssistantResponsePassesGroundedResponse(t *testing.T) {
	review := ReviewFinalAssistantResponse(FinalAssistantResponseReviewInput{
		UserInput: "what time is it in Virginia right now?",
		Response:  "Command: TZ=America/New_York date\nExit code: 0\nStdout: Wed May 20 10:00:00 EDT 2026",
		Evidence:  []string{"stdout=Wed May 20 10:00:00 EDT 2026"},
	})

	if !review.Passed {
		t.Fatalf("grounded response should pass: %#v", review)
	}
}
