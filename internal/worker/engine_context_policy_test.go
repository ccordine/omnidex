package worker

import (
	"encoding/json"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/specialist"
	"strings"
	"testing"
	"time"
)

func TestResolvePreparedPromptHintPreservesExplicitUserHint(t *testing.T) {
	want := "User request: create test-4 file in current directory"
	got := resolvePreparedPromptHint(
		"verify_evaluate_attempt_1_of_1_pass_1_of_1",
		"Instruction:\ncreate test-4",
		want,
	)
	if got != want {
		t.Fatalf("resolvePreparedPromptHint()=%q want %q", got, want)
	}
}

func TestExtractPromptLabelValue(t *testing.T) {
	prompt := strings.Join([]string{
		"Instruction:",
		"create a test file",
		"",
		"Plan:",
		"touch test",
	}, "\n")
	got := extractPromptLabelValue(prompt, "Instruction:", 120)
	if got != "create a test file" {
		t.Fatalf("extractPromptLabelValue()=%q want %q", got, "create a test file")
	}
}

func TestLocalClockOnlyHeuristics(t *testing.T) {
	if !isLocalClockOnlyInstruction("what time is it right now?") {
		t.Fatalf("expected local clock question to be detected")
	}
	if isLocalClockOnlyInstruction("what time is bitcoin price update today") {
		t.Fatalf("did not expect market/time-sensitive query to be classified as local clock only")
	}
}

func TestAnchorTimeSensitiveQuery(t *testing.T) {
	job := model.Job{
		Instruction: "latest nvidia stock news",
		Metadata:    json.RawMessage(`{"host_clock_local":"2026-02-15T12:00:00-05:00"}`),
	}
	got := anchorTimeSensitiveQuery("nvidia stock news", job)
	want := "nvidia stock news as of 2026-02-15"
	if got != want {
		t.Fatalf("anchorTimeSensitiveQuery()=%q, want %q", got, want)
	}
}

func TestForcePlanNeedsExternalInfo(t *testing.T) {
	plan := `{"goal":"x","tasks":["a"],"needs_external_info":false,"required_tools":[],"clarifications":[],"done_when":["ok"]}`
	updated := forcePlanNeedsExternalInfo(plan)
	needsExternal, decided := planNeedsExternalInfo(updated)
	if !decided {
		t.Fatalf("expected updated plan to include needs_external_info")
	}
	if !needsExternal {
		t.Fatalf("expected needs_external_info=true after override")
	}
}

func TestPersistentExecutionEnabled(t *testing.T) {
	enabled := model.Job{Metadata: json.RawMessage(`{"persistent_execution":"on"}`)}
	if !persistentExecutionEnabled(enabled) {
		t.Fatalf("expected persistent execution to be enabled")
	}

	disabled := model.Job{Metadata: json.RawMessage(`{"persistent_execution":"off"}`)}
	if persistentExecutionEnabled(disabled) {
		t.Fatalf("expected persistent execution to be disabled")
	}

	boolEnabled := model.Job{Metadata: json.RawMessage(`{"persistent_execution":true}`)}
	if !persistentExecutionEnabled(boolEnabled) {
		t.Fatalf("expected bool persistent execution to be enabled")
	}
}

func TestPlanningPassCount(t *testing.T) {
	if got := planningPassCount(model.Job{}); got != 3 {
		t.Fatalf("planningPassCount(default)=%d, want 3", got)
	}
	if got := planningPassCount(model.Job{Metadata: json.RawMessage(`{"planning_passes":1}`)}); got != 1 {
		t.Fatalf("planningPassCount(explicit)=%d, want 1", got)
	}
	if got := planningPassCount(model.Job{Metadata: json.RawMessage(`{"planning_passes":99}`)}); got != 5 {
		t.Fatalf("planningPassCount(clamped)=%d, want 5", got)
	}
}

func TestVerificationPassCount(t *testing.T) {
	if got := verificationPassCount(model.Job{}); got != 2 {
		t.Fatalf("verificationPassCount(default)=%d, want 2", got)
	}
	if got := verificationPassCount(model.Job{Metadata: json.RawMessage(`{"verification_passes":1}`)}); got != 2 {
		t.Fatalf("verificationPassCount(explicit override ignored)=%d, want 2", got)
	}
	if got := verificationPassCount(model.Job{Metadata: json.RawMessage(`{"verification_passes":99}`)}); got != 2 {
		t.Fatalf("verificationPassCount(clamped)=%d, want 2", got)
	}
}

func TestResolveMemoryRetrievalLimit(t *testing.T) {
	if got := resolveMemoryRetrievalLimit(model.Job{}, "what did we discuss earlier in this project?", "", 8); got <= 8 {
		t.Fatalf("resolveMemoryRetrievalLimit(lookback request)=%d, want >8", got)
	}
	if got := resolveMemoryRetrievalLimit(model.Job{}, "summarize current status", "", 8); got != 8 {
		t.Fatalf("resolveMemoryRetrievalLimit(default)=%d, want 8", got)
	}
	if got := resolveMemoryRetrievalLimit(model.Job{Metadata: json.RawMessage(`{"retrieval_limit":3}`)}, "anything", "", 8); got != 3 {
		t.Fatalf("resolveMemoryRetrievalLimit(metadata retrieval_limit)=%d, want 3", got)
	}
	if got := resolveMemoryRetrievalLimit(model.Job{Metadata: json.RawMessage(`{"memory_lookback":"deep"}`)}, "anything", "", 8); got <= 8 {
		t.Fatalf("resolveMemoryRetrievalLimit(memory_lookback deep)=%d, want >8", got)
	}
	if got := resolveMemoryRetrievalLimit(model.Job{Metadata: json.RawMessage(`{"memory_retrieval_limit":999}`)}, "anything", "", 8); got != 64 {
		t.Fatalf("resolveMemoryRetrievalLimit(clamped)=%d, want 64", got)
	}
}

func TestShouldDeepenMemoryLookback(t *testing.T) {
	if !shouldDeepenMemoryLookback("think back to older decisions on auth flow", "") {
		t.Fatalf("expected explicit lookback phrase to trigger deep retrieval")
	}
	if !shouldDeepenMemoryLookback("status update", "No, check earlier memory from last week") {
		t.Fatalf("expected feedback lookback phrase to trigger deep retrieval")
	}
	if shouldDeepenMemoryLookback("give me a quick current summary", "") {
		t.Fatalf("did not expect current summary request to trigger deep retrieval")
	}
}

func TestVerificationHallucinationRetryLimit(t *testing.T) {
	if got := verificationHallucinationRetryLimit(model.Job{}, 0); got != 2 {
		t.Fatalf("verificationHallucinationRetryLimit(default)=%d, want 2", got)
	}
	if got := verificationHallucinationRetryLimit(model.Job{}, 99); got != 6 {
		t.Fatalf("verificationHallucinationRetryLimit(clamped fallback)=%d, want 6", got)
	}
	if got := verificationHallucinationRetryLimit(model.Job{Metadata: json.RawMessage(`{"hallucination_retry_limit":4}`)}, 2); got != 4 {
		t.Fatalf("verificationHallucinationRetryLimit(metadata explicit)=%d, want 4", got)
	}
	if got := verificationHallucinationRetryLimit(model.Job{Metadata: json.RawMessage(`{"hallucination_retries":8}`)}, 2); got != 6 {
		t.Fatalf("verificationHallucinationRetryLimit(metadata clamped)=%d, want 6", got)
	}
	if got := verificationHallucinationRetryLimit(model.Job{Metadata: json.RawMessage(`{"hallucination_loop_limit":1}`)}, 2); got != 1 {
		t.Fatalf("verificationHallucinationRetryLimit(metadata alias)=%d, want 1", got)
	}
}

func TestHallucinationRetrySignal(t *testing.T) {
	outcome := verificationOutcome{Status: "retry", Summary: "needs retry"}
	if ok, _ := hallucinationRetrySignal("no_consensus statuses[pass=1,retry=1,blocked=1] total=3", nil, outcome); !ok {
		t.Fatalf("expected no_consensus to be treated as hallucination retry signal")
	}
	if ok, _ := hallucinationRetrySignal("", []string{"claims command/action execution without execution evidence in this run"}, outcome); !ok {
		t.Fatalf("expected unsupported execution claim signal to be treated as hallucination retry")
	}
	if ok, _ := hallucinationRetrySignal("", []string{"required action missing in this run: web_search"}, outcome); ok {
		t.Fatalf("did not expect required-action-missing signal to count as hallucination retry")
	}
	if ok, _ := hallucinationRetrySignal("", nil, verificationOutcome{Status: "pass"}); ok {
		t.Fatalf("did not expect non-retry outcomes to count as hallucination retry")
	}
}

func TestOllamaRestartCommandAttempts(t *testing.T) {
	job := model.Job{
		Metadata: json.RawMessage(`{"ollama_restart_command":"docker compose restart ollama || systemctl restart ollama"}`),
	}
	attempts := ollamaRestartCommandAttempts(job, "")
	if len(attempts) != 2 {
		t.Fatalf("expected metadata command chain to produce 2 attempts, got %d", len(attempts))
	}
	if got := commandLineLabel(attempts[0]); got != "docker compose restart ollama" {
		t.Fatalf("unexpected metadata first command: %q", got)
	}

	fromConfig := ollamaRestartCommandAttempts(model.Job{}, "service ollama restart || brew services restart ollama")
	if len(fromConfig) != 2 {
		t.Fatalf("expected configured command chain to produce 2 attempts, got %d", len(fromConfig))
	}
	if got := commandLineLabel(fromConfig[0]); got != "service ollama restart" {
		t.Fatalf("unexpected configured first command: %q", got)
	}

	defaults := ollamaRestartCommandAttempts(model.Job{}, "")
	if len(defaults) == 0 {
		t.Fatalf("expected default ollama restart attempts")
	}
	if got := commandLineLabel(defaults[0]); got != "docker compose restart ollama" {
		t.Fatalf("unexpected default first command: %q", got)
	}
}

func TestAggregateVerificationConsensusMajority(t *testing.T) {
	outcomes := []verificationOutcome{
		{Status: "pass", Confidence: 0.9, Summary: "pass one", Gaps: []string{"a"}},
		{Status: "pass", Confidence: 0.7, Summary: "pass two", Gaps: []string{"b"}},
		{Status: "retry", Confidence: 0.4, Summary: "retry one"},
	}
	consensus, ok, note := aggregateVerificationConsensus(outcomes, testReport{})
	if !ok {
		t.Fatalf("expected majority consensus, note=%q", note)
	}
	if consensus.Status != "pass" {
		t.Fatalf("expected majority pass, got %q", consensus.Status)
	}
	if consensus.Confidence < 0.79 || consensus.Confidence > 0.81 {
		t.Fatalf("expected averaged confidence near 0.80, got %.2f", consensus.Confidence)
	}
	if !strings.Contains(note, "majority=pass") {
		t.Fatalf("expected majority note, got %q", note)
	}
}

func TestAggregateVerificationConsensusNoMajorityTriggersRetry(t *testing.T) {
	outcomes := []verificationOutcome{
		{Status: "pass", Confidence: 0.9, Summary: "pass one"},
		{Status: "retry", Confidence: 0.6, Summary: "retry one"},
		{Status: "blocked", Confidence: 0.5, Summary: "blocked one"},
	}
	consensus, ok, note := aggregateVerificationConsensus(outcomes, testReport{})
	if ok {
		t.Fatalf("expected no majority consensus, note=%q", note)
	}
	if consensus.Status != "retry" {
		t.Fatalf("expected no-majority to force retry, got %q", consensus.Status)
	}
	if !strings.Contains(strings.ToLower(strings.Join(consensus.Gaps, " ")), "hallucination risk") {
		t.Fatalf("expected hallucination-risk gap, got %v", consensus.Gaps)
	}
	if !strings.Contains(note, "no_consensus") {
		t.Fatalf("expected no_consensus note, got %q", note)
	}
}

func TestAggregateVerificationConsensusDualConfirmation(t *testing.T) {
	bothPass := []verificationOutcome{
		{Status: "pass", Confidence: 0.9, Summary: "judge one pass"},
		{Status: "pass", Confidence: 0.7, Summary: "judge two pass"},
	}
	consensus, ok, note := aggregateVerificationConsensus(bothPass, testReport{})
	if !ok {
		t.Fatalf("expected dual pass consensus, note=%q", note)
	}
	if consensus.Status != "pass" {
		t.Fatalf("expected pass when both judges pass, got %q", consensus.Status)
	}
	if !strings.Contains(note, "dual_confirmation=yes") {
		t.Fatalf("expected dual confirmation note, got %q", note)
	}

	oneNo := []verificationOutcome{
		{Status: "pass", Confidence: 0.9, Summary: "judge one pass"},
		{Status: "retry", Confidence: 0.6, Summary: "judge two no"},
	}
	consensus, ok, note = aggregateVerificationConsensus(oneNo, testReport{})
	if ok {
		t.Fatalf("expected dual confirmation failure when one judge says no, note=%q", note)
	}
	if consensus.Status != "retry" {
		t.Fatalf("expected retry when both judges are not pass, got %q", consensus.Status)
	}
	if !strings.Contains(note, "dual_confirmation=no") {
		t.Fatalf("expected dual confirmation negative note, got %q", note)
	}
}

func TestHeuristicPlanSelectionPrefersExternalWhenForced(t *testing.T) {
	candidates := []string{
		`{"goal":"answer user","tasks":["read memory","reply"],"needs_external_info":false,"required_tools":[],"clarifications":[],"done_when":["user gets answer"]}`,
		`{"goal":"answer user with fresh data","tasks":["run web search","synthesize results"],"needs_external_info":true,"required_tools":["curl"],"clarifications":[],"done_when":["user gets current answer"]}`,
	}
	idx, _ := heuristicPlanSelection(candidates, "search the web for latest fed meeting statement", true)
	if idx != 1 {
		t.Fatalf("heuristicPlanSelection() index=%d, want 1", idx)
	}
}

func TestParseBestPlanIndex(t *testing.T) {
	raw := `{"best_index":2,"reason":"candidate 2 is most grounded"}`
	idx, reason, ok := parseBestPlanIndex(raw, 3)
	if !ok {
		t.Fatalf("expected parseBestPlanIndex to succeed")
	}
	if idx != 1 {
		t.Fatalf("parseBestPlanIndex index=%d, want 1", idx)
	}
	if !strings.Contains(reason, "grounded") {
		t.Fatalf("expected reason to be parsed, got %q", reason)
	}
}

func TestReviewAlwaysEnabled(t *testing.T) {
	if !reviewAlwaysEnabled(model.Job{}) {
		t.Fatalf("expected reviewAlwaysEnabled default true")
	}
	if reviewAlwaysEnabled(model.Job{Metadata: json.RawMessage(`{"review_always":"off"}`)}) {
		t.Fatalf("expected reviewAlwaysEnabled off override")
	}
}

func TestEnforceGroundingReviewTriggersRetry(t *testing.T) {
	outcome := verificationOutcome{
		Status:     "pass",
		Confidence: 0.9,
		Summary:    "looks good",
	}
	contexts := map[string]string{
		"web_search": "web search skipped: metadata mode=off",
	}
	updated, signals := enforceGroundingReview(
		outcome,
		model.Job{Instruction: "Find latest Nvidia stock price"},
		"I searched the web and found the latest price.",
		contexts,
		testReport{},
	)
	if len(signals) == 0 {
		t.Fatalf("expected grounding signals for unsupported web claim")
	}
	if updated.Status != "retry" {
		t.Fatalf("expected review to force retry, got %q", updated.Status)
	}
}

func TestMissingRequiredActionsForVerification(t *testing.T) {
	job := model.Job{
		Instruction: "Please do a web search for the latest Nvidia stock price",
		Metadata:    json.RawMessage(`{"web_search":"auto"}`),
	}
	contexts := map[string]string{
		"plan":       `{"goal":"x","tasks":["run web search"],"needs_external_info":true,"required_tools":[],"clarifications":[],"done_when":["ok"]}`,
		"web_search": "web search skipped: heuristic not triggered",
	}
	missing := missingRequiredActionsForVerification(job, contexts)
	if len(missing) != 1 || missing[0] != "web_search" {
		t.Fatalf("missingRequiredActionsForVerification()=%v, want [web_search]", missing)
	}

	contexts["web_search"] = "result[1]: Nvidia stock price as of 2026-02-15 ..."
	missing = missingRequiredActionsForVerification(job, contexts)
	if len(missing) != 0 {
		t.Fatalf("expected no missing required actions after web context, got %v", missing)
	}
}

func TestBuildVerificationActionAuditIncludesRequiredMissingActions(t *testing.T) {
	job := model.Job{
		Pipeline:    model.PipelineAssistant,
		Instruction: "Search the web for latest CUDA release notes",
		Metadata:    json.RawMessage(`{"web_search":"auto"}`),
	}
	contexts := map[string]string{
		"plan":       `{"goal":"x","tasks":["run web search"],"needs_external_info":true,"required_tools":[],"clarifications":[],"done_when":["ok"]}`,
		"web_search": "web search skipped: metadata mode=off",
		"assist":     "Draft response without web context",
	}
	audit := buildVerificationActionAudit(job, contexts)
	if !strings.Contains(audit.Report, "web_search=skipped") {
		t.Fatalf("expected web_search skipped status in audit, got: %q", audit.Report)
	}
	if !strings.Contains(audit.Report, "required_missing_actions=web_search") {
		t.Fatalf("expected required missing action in audit, got: %q", audit.Report)
	}
}

func TestCountAutoVerifyReplans(t *testing.T) {
	contexts := []model.StepContext{
		{Key: "replan_feedback", Value: "manual user correction"},
		{Key: "replan_feedback", Value: "auto_verify_replan: restart from planning because required actions were missed."},
		{Key: "user_feedback", Value: "auto_verify_replan: duplicate source should not count"},
	}
	if got := countAutoVerifyReplans(contexts); got != 1 {
		t.Fatalf("countAutoVerifyReplans()=%d, want 1", got)
	}
}

func TestAutoVerifyReplanFeedbackTriggersForAnyNonPassInPersistentMode(t *testing.T) {
	job := model.Job{
		Instruction: "Implement feature X and verify it works",
		Metadata:    json.RawMessage(`{"persistent_execution":"on"}`),
	}
	contexts := map[string]string{
		"verification_action_audit": "plan=executed\ntooling=executed\nverify=executed",
	}
	outcome := verificationOutcome{
		Status:  "retry",
		Summary: "judge disagreement on completion",
		Gaps:    []string{"missing final behavior assertion"},
	}

	feedback, missing, ok := autoVerifyReplanFeedback(job, contexts, nil, outcome)
	if !ok {
		t.Fatalf("expected auto replan feedback to trigger")
	}
	if len(missing) != 0 {
		t.Fatalf("expected no missing required actions in this scenario, got %v", missing)
	}
	if !strings.Contains(feedback, "replan_mode=objective_recovery") {
		t.Fatalf("expected objective recovery marker in feedback, got %q", feedback)
	}
	if !strings.Contains(feedback, "verification_gaps=missing final behavior assertion") {
		t.Fatalf("expected verification gap in feedback, got %q", feedback)
	}
}

func TestResponseSeemsOffTopic(t *testing.T) {
	if !responseSeemsOffTopic(
		"install omnidex and set aliases at boot for this user",
		"The weather in Paris is mild with scattered clouds and light wind today.",
	) {
		t.Fatalf("expected off-topic response detection")
	}
	if responseSeemsOffTopic(
		"install omnidex and set aliases at boot for this user",
		"I can install omnidex under ~/.omnidex and configure shell aliases loaded at login.",
	) {
		t.Fatalf("did not expect on-topic response to be flagged")
	}
}

func TestSanitizeSearchQueryArtifacts(t *testing.T) {
	got := sanitizeSearchQueryArtifacts("nvidia stock news as of CURRENT_TIME_CONTEXT as of 2026-02-15")
	want := "nvidia stock news as of 2026-02-15"
	if got != want {
		t.Fatalf("sanitizeSearchQueryArtifacts()=%q, want %q", got, want)
	}
}

func TestShouldAttachRecentConversation(t *testing.T) {
	job := model.Job{
		Pipeline: model.PipelineChat,
		Metadata: json.RawMessage(`{"session_id":"chat-123"}`),
	}
	if !shouldAttachRecentConversation(job, "analyze") {
		t.Fatalf("expected analyze step to attach recent conversation")
	}
	if shouldAttachRecentConversation(model.Job{Pipeline: model.PipelineAssistant, Metadata: json.RawMessage(`{"session_id":"chat-123"}`)}, "analyze") {
		t.Fatalf("did not expect non-chat pipeline to attach recent conversation")
	}
	if shouldAttachRecentConversation(model.Job{Pipeline: model.PipelineChat}, "analyze") {
		t.Fatalf("did not expect missing session_id to attach recent conversation")
	}
}

func TestProjectTag(t *testing.T) {
	job := model.Job{
		Metadata: json.RawMessage(`{"client_cwd":"/home/gryph/Projects/ai/omnidex"}`),
	}
	tag := projectTag(job)
	if !strings.HasPrefix(tag, "project:omnidex-") {
		t.Fatalf("expected project tag prefix, got %q", tag)
	}
	if len(tag) <= len("project:omnidex-") {
		t.Fatalf("expected hash suffix in project tag, got %q", tag)
	}
	if tag != projectTag(job) {
		t.Fatalf("expected deterministic project tag, got %q and %q", tag, projectTag(job))
	}

	other := model.Job{
		Metadata: json.RawMessage(`{"client_cwd":"/home/gryph/Projects/ai/another-repo"}`),
	}
	if projectTag(other) == tag {
		t.Fatalf("expected different project tag for different cwd")
	}

	fallback := model.Job{
		Metadata: json.RawMessage(`{"host_env_cwd":"/tmp/workspace-x"}`),
	}
	if !strings.HasPrefix(projectTag(fallback), "project:workspace-x-") {
		t.Fatalf("expected host_env_cwd fallback project tag, got %q", projectTag(fallback))
	}
}

func TestMemoryScopeTags(t *testing.T) {
	job := model.Job{
		Metadata: json.RawMessage(`{"session_id":"chat-123","client_cwd":"/tmp/omnidex"}`),
	}
	tags := memoryScopeTags(job, []string{"build", "session:chat-123"})
	joined := strings.Join(tags, ",")
	if !strings.Contains(joined, "project:omnidex-") {
		t.Fatalf("expected project-scoped tag in %v", tags)
	}
	if !strings.Contains(joined, "session:chat-123") {
		t.Fatalf("expected session tag in %v", tags)
	}
	if strings.Count(joined, "session:chat-123") != 1 {
		t.Fatalf("expected deduplicated session tag, got %v", tags)
	}
}

func TestRankMemoryOmnibusMatchesPrioritizesRecentSessionContext(t *testing.T) {
	now := time.Date(2026, 2, 15, 19, 0, 0, 0, time.UTC)
	scopeTags := []string{"project:omnidex-1234abcd", "session:chat-123", "vlc", "memory"}

	matches := []model.MemoryMatch{
		{
			ID:        1,
			Kind:      model.MemoryKindProcedural,
			Content:   "Steps to configure VLC keybindings.",
			Tags:      []string{"project:omnidex-1234abcd", "memory"},
			Score:     0.95,
			CreatedAt: now.Add(-7 * 24 * time.Hour),
		},
		{
			ID:        2,
			Kind:      model.MemoryKindEpisodic,
			Content:   "You just asked what is currently playing in VLC.",
			Tags:      []string{"project:omnidex-1234abcd", "session:chat-123", "vlc"},
			Score:     0.55,
			CreatedAt: now.Add(-5 * time.Minute),
		},
		{
			ID:        3,
			Kind:      model.MemoryKindReference,
			Content:   "VLC MPRIS metadata fields and meanings.",
			Tags:      []string{"project:omnidex-1234abcd", "vlc"},
			Score:     0.80,
			CreatedAt: now.Add(-48 * time.Hour),
		},
	}

	ranked := rankMemoryOmnibusMatches(
		matches,
		"what did we just say about vlc in this chat?",
		scopeTags,
		"project:omnidex-1234abcd",
		"session:chat-123",
		2,
		now,
	)
	if len(ranked) != 2 {
		t.Fatalf("expected top 2 matches, got %d", len(ranked))
	}
	if ranked[0].ID != 2 {
		t.Fatalf("expected recent session episodic memory first, got id=%d", ranked[0].ID)
	}
}

func TestDeriveRelatedMemoryTags(t *testing.T) {
	scopeTags := []string{"browser:email", "urgent"}
	matches := []model.MemoryMatch{
		{
			ID:   1,
			Tags: []string{"browser:tabs", "browser:gmail", "project:omnidex-1234abcd"},
		},
		{
			ID:   2,
			Tags: []string{"urgent-response", "notifications", "session:chat-abc"},
		},
	}

	related := deriveRelatedMemoryTags(scopeTags, matches, 5)
	joined := strings.Join(related, ",")
	if !strings.Contains(joined, "browser:tabs") && !strings.Contains(joined, "browser:gmail") {
		t.Fatalf("expected browser-family related tags, got %v", related)
	}
	if !strings.Contains(joined, "urgent-response") {
		t.Fatalf("expected substring-related urgent tag, got %v", related)
	}
	if strings.Contains(joined, "project:") || strings.Contains(joined, "session:") {
		t.Fatalf("did not expect project/session tags in related set, got %v", related)
	}
}

func TestBuildRetrievalContextIncludesCreatedAt(t *testing.T) {
	now := time.Date(2026, 2, 15, 20, 30, 0, 0, time.UTC)
	output := buildRetrievalContext([]model.MemoryMatch{
		{
			ID:        77,
			Kind:      model.MemoryKindEpisodic,
			Content:   "Recent context",
			Tags:      []string{"session:chat-123"},
			Score:     0.71,
			CreatedAt: now,
		},
	}, 2000)

	if !strings.Contains(output, "created_at=2026-02-15T20:30:00Z") {
		t.Fatalf("expected created_at in retrieval context, got %q", output)
	}
}

func TestFormatRecentConversationTurns(t *testing.T) {
	turns := []model.Job{
		{ID: 10, Status: model.JobStatusCompleted, Instruction: "Can you create a file?", Result: "Run `touch test` in the current directory."},
		{ID: 11, Status: model.JobStatusCompleted, Instruction: "Did you do it?", Result: "I only suggested the command; it was not executed."},
	}

	got := formatRecentConversationTurns(turns, 1200)
	if got == "" {
		t.Fatalf("expected formatted conversation context")
	}
	if !strings.Contains(got, "turn_id=10") || !strings.Contains(got, "turn_id=11") {
		t.Fatalf("expected turn ids in formatted output, got: %q", got)
	}
	if !strings.Contains(got, "user: Can you create a file?") {
		t.Fatalf("expected user line in formatted output, got: %q", got)
	}
	if !strings.Contains(got, "assistant: I only suggested the command; it was not executed.") {
		t.Fatalf("expected assistant line in formatted output, got: %q", got)
	}
}

func TestHasRetrievalContextSkipsFreshContextMarker(t *testing.T) {
	if hasRetrievalContext("Historical memory retrieval skipped: fresh context requested for this turn.") {
		t.Fatalf("expected fresh-context retrieval skip marker to be treated as non-context")
	}
}

func TestEnsureResponseHasSourcesAppendsSection(t *testing.T) {
	job := model.Job{
		Pipeline: model.PipelineChat,
		Metadata: json.RawMessage(`{"session_id":"chat-abc"}`),
	}
	contexts := map[string]string{
		"recent_conversation": "turn_id=1 status=completed\nuser: hi\nassistant: hello",
		"retrieval":           "[1] kind=episodic score=0.7 tags=session\nprevious context",
		"web_search":          "search provider: example result context",
	}

	got := ensureResponseHasSources("Here is the answer.", job, contexts, nil)
	if !strings.Contains(got, "\nSources:\n") {
		t.Fatalf("expected Sources section, got: %q", got)
	}
	if !strings.Contains(got, "- user_instruction: current turn input") {
		t.Fatalf("expected user instruction source line, got: %q", got)
	}
	if !strings.Contains(got, "- recent_conversation: recent turns from session chat-abc") {
		t.Fatalf("expected recent conversation source line, got: %q", got)
	}
	if !strings.Contains(got, "- retrieved_memory: memory retrieval context from this run") {
		t.Fatalf("expected retrieval source line, got: %q", got)
	}
	if !strings.Contains(got, "- web_search: externally fetched context from this run") {
		t.Fatalf("expected web source line, got: %q", got)
	}
}

func TestEnsureResponseHasSourcesDoesNotDuplicate(t *testing.T) {
	job := model.Job{}
	contexts := map[string]string{}
	input := "Answer body.\n\nSources:\n- user_instruction: current turn input"
	got := ensureResponseHasSources(input, job, contexts, nil)
	if got != input {
		t.Fatalf("expected response with existing source section to remain unchanged")
	}
}

func TestBuildResponseSourceLinesIncludesTestExecution(t *testing.T) {
	lines := buildResponseSourceLines(model.Job{}, map[string]string{}, &testReport{
		Attempted: 2,
		Passed:    2,
	})
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "- executed_tests: commands listed in Executed Test Evidence") {
		t.Fatalf("expected executed tests source line, got: %q", joined)
	}
}

func TestPipelinePhaseForActionPlanningResearchActions(t *testing.T) {
	for _, action := range []string{"tooling", "workspace_scan", "tag", "retrieve", "plan"} {
		if got := pipelinePhaseForAction(action); got != "planning" {
			t.Fatalf("pipelinePhaseForAction(%q)=%q want planning", action, got)
		}
	}
	if got := pipelinePhaseForAction("verify"); got != "review" {
		t.Fatalf("pipelinePhaseForAction(verify)=%q want review", got)
	}
	if got := pipelinePhaseForAction("assist"); got != "execution" {
		t.Fatalf("pipelinePhaseForAction(assist)=%q want execution", got)
	}
}

func TestSpecialistModelUsesRoleOverride(t *testing.T) {
	svc := Service{
		models: ModelRouting{
			Specialist: map[string]string{
				specialist.RoleBrowserInspectionSpecialist: "browser-model",
			},
		},
	}

	job := model.Job{Metadata: json.RawMessage(`{"specialist_role_id":"browser_inspection_specialist"}`)}
	got := svc.specialistModel(job, specialist.RoleResponseSpecialist, "fallback-model")
	if got != "browser-model" {
		t.Fatalf("specialistModel()=%q want browser-model", got)
	}

	job = model.Job{}
	got = svc.specialistModel(job, specialist.RoleResponseSpecialist, "fallback-model")
	if got != "fallback-model" {
		t.Fatalf("specialistModel() fallback=%q want fallback-model", got)
	}
}
