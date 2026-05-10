package main

import (
	"strings"
	"testing"
	"time"
)

func TestFormatLocalAutomationResponseAddsSource(t *testing.T) {
	got := formatLocalAutomationResponse("local_shell: local command execution output", "Executed: pwd")
	if !strings.Contains(got, "\nSources:\n- local_shell: local command execution output") {
		t.Fatalf("expected source section to be appended, got: %q", got)
	}
}

func TestFormatLocalAutomationResponseDoesNotDuplicate(t *testing.T) {
	input := "Executed: pwd\n\nSources:\n- local_shell: local command execution output"
	got := formatLocalAutomationResponse("local_shell: local command execution output", input)
	if got != input {
		t.Fatalf("expected existing source section to remain unchanged")
	}
}

func TestBuildActionReinterpretationPromptIncludesContext(t *testing.T) {
	candidate := &chatActionCandidate{
		Kind:    "local_media",
		Input:   "Can you tell me what's currently playing on VLC right now?",
		Summary: "resume/play the active local media player (VLC via MPRIS/playerctl)",
	}
	got := buildActionReinterpretationPrompt(
		candidate,
		"I asked for what's playing, not to press play.",
		true,
		true,
		true,
		true,
		true,
	)
	for _, want := range []string{
		"Original request:",
		candidate.Input,
		"Rejected interpretation:",
		candidate.Summary,
		"Assigned specialist:",
		"media_control_specialist",
		"Use recent conversation from this chat session to preserve context.",
		"Available local capabilities:",
		"local_shell: run local shell commands",
		"Safety constraints:",
		"If elevated access is required, ask for sudo and explain why",
		"Do not remove/delete files.",
		"User feedback on what was wrong:",
		"Questions:",
		"Confirmation:",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q: %q", want, got)
		}
	}
}

func TestBuildActionReinterpretationPromptNoFeedbackAsksClarifyingQuestions(t *testing.T) {
	candidate := &chatActionCandidate{
		Kind:    "local_media",
		Input:   "what's playing in vlc?",
		Summary: "play the active local media player",
	}
	got := buildActionReinterpretationPrompt(candidate, "", true, true, true, true, true)
	for _, want := range []string{
		"User feedback on what was wrong:",
		"(none provided; user said no without details)",
		"Ask 2-4 targeted clarifying questions",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q: %q", want, got)
		}
	}
}

func TestBuildActionInterpretationPromptIncludesRecentConversationInstruction(t *testing.T) {
	candidate := &chatActionCandidate{
		Kind:    "core_job",
		Input:   "help me review this repo",
		Summary: "submit this request to the core pipeline",
	}
	got := buildActionInterpretationPrompt(candidate, true, true, true, true, true)
	for _, want := range []string{
		"Interpret the user's request before execution.",
		"Use recent conversation from this chat session to preserve context.",
		"Original request:",
		candidate.Input,
		"Preliminary routing guess:",
		"Assigned specialist:",
		"Interpretation:",
		"Questions:",
		"Confirmation:",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q: %q", want, got)
		}
	}
}

func TestLocalAutomationSourceLineIncludesSpecialist(t *testing.T) {
	got := localAutomationSourceLine("local_browser", "local browser process/tab/console inspection output")
	if !strings.Contains(got, "local_browser") {
		t.Fatalf("expected kind in source line, got: %q", got)
	}
	if !strings.Contains(got, "browser_inspection_specialist") {
		t.Fatalf("expected specialist id in source line, got: %q", got)
	}
}

func TestSpecialistRoleIDForCandidateTurn(t *testing.T) {
	local := &chatActionCandidate{
		Kind:         "local_media",
		SpecialistID: "media_control_specialist",
	}
	if got := specialistRoleIDForCandidateTurn(local); got != "media_control_specialist" {
		t.Fatalf("specialistRoleIDForCandidateTurn(local)=%q", got)
	}

	core := &chatActionCandidate{
		Kind:         "core_job",
		SpecialistID: "planner_specialist",
	}
	if got := specialistRoleIDForCandidateTurn(core); got != "" {
		t.Fatalf("specialistRoleIDForCandidateTurn(core)=%q want empty", got)
	}
}

func TestApplySpecialistTurnOverridesAppliesEnvModelWithoutOverwriting(t *testing.T) {
	t.Setenv("OLLAMA_MODEL_SPECIALIST_BROWSER_INSPECTION", "browser-specialist-model")
	metadata := map[string]any{
		"model_response": "already-set",
	}

	applySpecialistTurnOverrides(metadata, "browser_inspection_specialist")
	if got := strings.TrimSpace(metadata["specialist_role_id"].(string)); got != "browser_inspection_specialist" {
		t.Fatalf("specialist_role_id=%q", got)
	}
	if got := strings.TrimSpace(metadata["model_plan"].(string)); got != "browser-specialist-model" {
		t.Fatalf("model_plan=%q", got)
	}
	if got := strings.TrimSpace(metadata["model_response"].(string)); got != "already-set" {
		t.Fatalf("model_response=%q want already-set", got)
	}
	if _, ok := metadata["model_tagger"]; ok {
		t.Fatalf("model_tagger should not be overridden by specialist model")
	}
	if _, ok := metadata["model_search"]; ok {
		t.Fatalf("model_search should not be overridden by specialist model")
	}
	if _, ok := metadata["model_memory"]; ok {
		t.Fatalf("model_memory should not be overridden by specialist model")
	}
}

func TestInterpretConfirmationReply(t *testing.T) {
	decision, feedback := interpretConfirmationReply("yes")
	if decision != confirmationDecisionApprove || feedback != "" {
		t.Fatalf("expected yes -> approve, got decision=%s feedback=%q", decision, feedback)
	}
	decision, feedback = interpretConfirmationReply("yes, sounds good")
	if decision != confirmationDecisionApprove || feedback != "" {
		t.Fatalf("expected yes-with-punctuation -> approve, got decision=%s feedback=%q", decision, feedback)
	}
	decision, feedback = interpretConfirmationReply("yes but rename it to daughter")
	if decision != confirmationDecisionRevise || feedback != "yes but rename it to daughter" {
		t.Fatalf("expected yes-but -> revise, got decision=%s feedback=%q", decision, feedback)
	}

	decision, feedback = interpretConfirmationReply("no")
	if decision != confirmationDecisionReject || feedback != "" {
		t.Fatalf("expected no -> reject without feedback, got decision=%s feedback=%q", decision, feedback)
	}

	decision, feedback = interpretConfirmationReply("no, but I meant check status only")
	if decision != confirmationDecisionReject || feedback != "I meant check status only" {
		t.Fatalf("expected no-but -> reject with extracted feedback, got decision=%s feedback=%q", decision, feedback)
	}

	decision, feedback = interpretConfirmationReply("Use Firefox and check Gmail")
	if decision != confirmationDecisionRevise || feedback != "Use Firefox and check Gmail" {
		t.Fatalf("expected freeform -> revise feedback, got decision=%s feedback=%q", decision, feedback)
	}
}

func TestRequiresActionConfirmation(t *testing.T) {
	if requiresActionConfirmation(false, &chatActionCandidate{Kind: "local_shell"}) {
		t.Fatal("expected disabled confirm-actions to skip confirmation")
	}
	if requiresActionConfirmation(true, nil) {
		t.Fatal("expected nil candidate to skip confirmation")
	}
	if requiresActionConfirmation(true, &chatActionCandidate{Kind: "core_job"}) {
		t.Fatal("expected core jobs to skip local action confirmation")
	}
	if !requiresActionConfirmation(true, &chatActionCandidate{Kind: "local_shell", Input: "create a test file"}) {
		t.Fatal("expected local shell actions to require confirmation and AI interpretation")
	}
	if !requiresActionConfirmation(true, &chatActionCandidate{Kind: "local_shell", Input: "install network tools"}) {
		t.Fatal("expected local shell actions to require confirmation")
	}
	if !requiresActionConfirmation(true, &chatActionCandidate{Kind: "local_media", Input: "what is currently playing in vlc?"}) {
		t.Fatal("expected local media actions to require confirmation and AI interpretation")
	}
	if !requiresActionConfirmation(true, &chatActionCandidate{Kind: "local_media", Input: "pause vlc playback"}) {
		t.Fatal("expected local media actions to require confirmation")
	}
}

func TestBuildChatActionCandidateRoutesComplexCreateFileRequestToCoreJob(t *testing.T) {
	candidate := buildChatActionCandidate(
		"Using tailwind css, make an index.html file and build a sexy hello world landing page",
		true,
		true,
		true,
		true,
		true,
		&localShellState{},
	)
	if candidate == nil {
		t.Fatal("expected candidate")
	}
	if candidate.Kind != "core_job" {
		t.Fatalf("expected core_job for complex authoring request, got %q", candidate.Kind)
	}
}

func TestBuildChatActionCandidateKeepsExistenceCheckCreateInLocalShell(t *testing.T) {
	candidate := buildChatActionCandidate(
		"Is there a test file present in the current directory? If not, create a new file called `test` and name it `index.html`.",
		true,
		true,
		true,
		true,
		true,
		&localShellState{},
	)
	if candidate == nil {
		t.Fatal("expected candidate")
	}
	if candidate.Kind != "local_shell" {
		t.Fatalf("expected local_shell for direct file check/create request, got %q", candidate.Kind)
	}
}

func TestBuildChatActionCandidateKeepsCurrentDirectoryIndexCreateInLocalShell(t *testing.T) {
	candidate := buildChatActionCandidate(
		"okay, in this current directory, let's make a test index.html",
		true,
		true,
		true,
		true,
		true,
		&localShellState{},
	)
	if candidate == nil {
		t.Fatal("expected candidate")
	}
	if candidate.Kind != "local_shell" {
		t.Fatalf("expected local_shell for direct index creation request, got %q", candidate.Kind)
	}
	if !strings.Contains(candidate.Summary, "index.html") {
		t.Fatalf("expected summary to reference index.html, got %q", candidate.Summary)
	}
}

func TestRevisedChatActionCandidateKeepsOriginalCoreRequest(t *testing.T) {
	previous := &chatActionCandidate{
		Kind:    "core_job",
		Input:   "Hello",
		Summary: "submit this request to the core pipeline (`Hello`) and run planning -> execution -> review",
	}
	got := revisedChatActionCandidate(
		previous,
		"no I was just saying hello, you picked up older information",
		true,
		true,
		true,
		true,
		true,
		&localShellState{},
	)
	if got == nil {
		t.Fatal("expected revised candidate")
	}
	if got.Input != "Hello" {
		t.Fatalf("expected original core request to be preserved, got %q", got.Input)
	}
	if got.Kind != "core_job" {
		t.Fatalf("expected core_job kind, got %q", got.Kind)
	}
}

func TestParseQueuedTurnInput(t *testing.T) {
	msg, ok := parseQueuedTurnInput("\tfollow up with test details")
	if !ok || msg != "follow up with test details" {
		t.Fatalf("expected tab-prefixed line to queue, got ok=%t msg=%q", ok, msg)
	}

	msg, ok = parseQueuedTurnInput("\t   ")
	if ok || msg != "" {
		t.Fatalf("expected empty tab-prefixed line to be ignored, got ok=%t msg=%q", ok, msg)
	}

	msg, ok = parseQueuedTurnInput("follow up without tab")
	if ok || msg != "" {
		t.Fatalf("expected non-tab line to not queue, got ok=%t msg=%q", ok, msg)
	}
}

func TestAdoptFreshLocalShellSuggestionCandidateUsesNewSuggestedCommand(t *testing.T) {
	candidate := &chatActionCandidate{
		Kind:    "local_shell",
		Input:   "create a test file",
		Summary: "create file `test` in the current directory",
	}
	before := time.Now().Add(-2 * time.Second)
	state := &localShellState{
		LastSuggestedCommand: "touch test",
		LastSuggestedAt:      time.Now(),
	}

	got := adoptFreshLocalShellSuggestionCandidate(candidate, state, "", before)
	if got == nil {
		t.Fatal("expected candidate")
	}
	if got.Input != "touch test" {
		t.Fatalf("input=%q want %q", got.Input, "touch test")
	}
	if !strings.Contains(got.Summary, "`touch test`") {
		t.Fatalf("summary=%q", got.Summary)
	}
}

func TestAdoptFreshLocalShellSuggestionCandidateSkipsStaleOrSameSuggestion(t *testing.T) {
	candidate := &chatActionCandidate{
		Kind:    "local_shell",
		Input:   "create a test file",
		Summary: "create file `test` in the current directory",
	}
	unchangedAt := time.Now()
	state := &localShellState{
		LastSuggestedCommand: "touch test",
		LastSuggestedAt:      unchangedAt,
	}

	got := adoptFreshLocalShellSuggestionCandidate(candidate, state, "touch test", unchangedAt)
	if got == nil {
		t.Fatal("expected candidate")
	}
	if got.Input != candidate.Input {
		t.Fatalf("expected stale/same suggestion to keep input=%q got=%q", candidate.Input, got.Input)
	}
}

func TestAdoptFreshLocalShellSuggestionCandidateOnlyAppliesToLocalShell(t *testing.T) {
	candidate := &chatActionCandidate{
		Kind:    "core_job",
		Input:   "help me debug this",
		Summary: "submit this request to the core pipeline",
	}
	before := time.Now().Add(-2 * time.Second)
	state := &localShellState{
		LastSuggestedCommand: "pwd",
		LastSuggestedAt:      time.Now(),
	}

	got := adoptFreshLocalShellSuggestionCandidate(candidate, state, "", before)
	if got == nil {
		t.Fatal("expected candidate")
	}
	if got.Input != candidate.Input {
		t.Fatalf("core job candidate should not be rewritten: got=%q want=%q", got.Input, candidate.Input)
	}
}

func TestBuildDeterministicLocalActionReviewPromptIncludesRequiredContract(t *testing.T) {
	candidate := &chatActionCandidate{
		Kind:    "local_shell",
		Input:   "create a test file",
		Summary: "create file `test` in the current directory",
	}
	got := buildDeterministicLocalActionReviewPrompt(candidate, "Executed: touch test")
	for _, want := range []string{
		"Deterministic post-action review step (required):",
		"Do not skip this review.",
		"`INCOMPLETE:`",
		"`COMPLETE:`",
		"Original user request:",
		"create a test file",
		"Executed local action output:",
		"Executed: touch test",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q: %q", want, got)
		}
	}
}

func TestDeterministicLocalActionReviewMetadataOverrides(t *testing.T) {
	base := map[string]any{
		"verification_mode":       "off",
		"verification_iterations": 1,
		"approval_mode":           "force",
	}
	review := deterministicLocalActionReviewMetadata(base)

	if got := strings.TrimSpace(review["verification_mode"].(string)); got != "force" {
		t.Fatalf("verification_mode=%q want force", got)
	}
	if got := review["verification_iterations"].(int); got < 2 {
		t.Fatalf("verification_iterations=%d want >=2", got)
	}
	if got, ok := review["review_always"].(bool); !ok || !got {
		t.Fatalf("review_always=%v want true", review["review_always"])
	}
	if got := strings.TrimSpace(review["approval_mode"].(string)); got != "force" {
		t.Fatalf("unrelated metadata should be preserved, approval_mode=%q", got)
	}
}
