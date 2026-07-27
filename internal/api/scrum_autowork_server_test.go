package api

import (
	"context"
	"os"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestScrumRequestForProjectCarriesProjectID(t *testing.T) {
	req := scrumRequestForProject(context.Background(), 42)
	if req == nil || req.URL == nil {
		t.Fatal("expected request URL")
	}
	if got := req.URL.Query().Get("project_id"); got != "42" {
		t.Fatalf("project_id=%q want 42", got)
	}
}

func TestJobFinishedAutoWorkHandoffStaysServerSide(t *testing.T) {
	source := readAPISource(t, "scrum_play_autorun.go")
	if strings.Contains(source, "s.RefreshScrumAutoWorkAsync()") {
		t.Fatal("job-finished handoff must call global auto-work in the same backend goroutine")
	}
	if !strings.Contains(source, "s.refreshScrumAutoWork(ctx)") {
		t.Fatal("expected job-finished handoff to call refreshScrumAutoWork(ctx)")
	}
}

func TestRefreshScrumPlayQueuePerformsServerAutoWorkHandoff(t *testing.T) {
	source := readAPISource(t, "scrum_play_queue.go")
	required := "return s.kickoffAutoWorkAfterReconcile(r, projectID, board)"
	if !strings.Contains(source, required) {
		t.Fatalf("scrum play reconcile must hand off to server auto-work; missing %q", required)
	}
}

func TestReconcileOnlyAutoWorkHandoffCanBeSuppressed(t *testing.T) {
	autorun := readAPISource(t, "scrum_play_autorun.go")
	if !strings.Contains(autorun, "if !advanceAutoWork {") ||
		!strings.Contains(autorun, "scrumAutoWorkHandoffSuppressedKey{}") {
		t.Fatal("reconcile-only queue refresh must mark auto-work handoff as suppressed")
	}
	autoPlay := readAPISource(t, "scrum_auto_play.go")
	if !strings.Contains(autoPlay, "scrumAutoWorkHandoffSuppressed(r.Context())") {
		t.Fatal("auto-work handoff must honor reconcile-only suppression")
	}
}

func TestStartScrumCardPlayUsesAuthoritativeProjectID(t *testing.T) {
	source := readAPISource(t, "scrum_play_queue.go")
	body := sourceBetween(t, source, "func (s *Server) startScrumCardPlay", "\nfunc scrumCardFromBoard")
	if !strings.Contains(body, "s.scrumBoardFromProject(r.Context(), projectID)") {
		t.Fatal("startScrumCardPlay must load project board from the server-side project ID")
	}
	if strings.Contains(body, "card, board, projectID, err := s.scrumGetCard(r, cardID)") {
		t.Fatal("startScrumCardPlay must not depend on request URL project resolution when projectID is known")
	}
}

func TestAutoWorkKickoffDoesNotSilentlySwallowStartFailures(t *testing.T) {
	source := readAPISource(t, "scrum_auto_play.go")
	required := []string{
		"markScrumAutoWorkStartFailure",
		"moving_to=error",
	}
	for _, snippet := range required {
		if !strings.Contains(source, snippet) {
			t.Fatalf("expected auto-work start failures to be explicit; missing %q", snippet)
		}
	}
	forbidden := []string{
		"if _, err := s.startScrumCardPlay(r, board, projectID, prepared.ID, agentconfig.Config{}); err != nil {\n\t\treturn board, nil\n\t}",
		"if err != nil {\n\t\treturn board, nil\n\t}\n\tif _, err := s.startScrumCardPlay",
	}
	for _, snippet := range forbidden {
		if strings.Contains(source, snippet) {
			t.Fatalf("auto-work must not swallow start failures; found %q", snippet)
		}
	}
}

func TestScrumAutoWorkStartErrorIsGlobalPause(t *testing.T) {
	if !scrumAutoWorkStartErrorIsGlobalPause(autoWorkErrString("AI is globally paused")) {
		t.Fatal("expected global pause detection")
	}
	if scrumAutoWorkStartErrorIsGlobalPause(autoWorkErrString("spawn codex ENOENT")) {
		t.Fatal("agent start failures must not be treated as global pause")
	}
}

func TestAutoWorkFailureReasonTruncationKeepsValidUTF8(t *testing.T) {
	reason := strings.Repeat("a", scrumAutoWorkStartFailureNoteLimit) + "→ failed"
	got := truncateScrumChannelText(reason, scrumAutoWorkStartFailureNoteLimit+2, "...")
	if !utf8.ValidString(got) {
		t.Fatalf("not valid utf8: %q", got)
	}
	if strings.Contains(got, string([]byte{0xe2, 0x86, 0x2e})) {
		t.Fatalf("found truncated arrow before suffix: %q", got)
	}
}

func TestGlobalAutoWorkDoesNotSkipAuthoritativeFailures(t *testing.T) {
	source := readAPISource(t, "scrum_global_autowork.go")
	for _, forbidden := range []string{
		"scrum global auto-work load project=%d",
		"scrum global running reconcile project=%d",
		"if board, err := s.scrumBoardFromProject(ctx, candidate.projectID); err == nil",
		"if refreshed, err := s.kickoffAutoPlayThrough",
		"return true\n\t}\n\treturn running",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("global Scrum auto-work contains a hidden recovery path %q", forbidden)
		}
	}
}

func TestAIControlDoesNotSwallowPauseOrResumeFailures(t *testing.T) {
	source := readAPISource(t, "ai_control.go")
	for _, forbidden := range []string{
		"projectIDs, _ :=",
		"if _, err := s.repo.CancelJob(ctx, jobID, \"global AI pause\"); err == nil",
		"_ = s.refreshScrumPlayQueueForProject",
		"s.RefreshScrumAutoWorkAsync()\n\treturn",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("AI control contains a swallowed orchestration failure %q", forbidden)
		}
	}
}

func TestAutoReviewDoesNotInventAVerdictOrIgnoreTelemetryFailure(t *testing.T) {
	source := readAPISource(t, "scrum_auto_review.go")
	for _, forbidden := range []string{
		"completed without explicit verdict",
		"job did not complete — bouncing",
		"_ = s.repo.RecordScrumFlowEvent",
		"payload, _ := json.Marshal",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("Scrum auto-review contains a hidden fallback %q", forbidden)
		}
	}
}

func TestScrumFlowMetricsLogsEveryPersistenceFailure(t *testing.T) {
	source := readAPISource(t, "scrum_flow_metrics.go")
	for _, forbidden := range []string{
		"_ = s.repo.RecordScrumFlowEvent",
		"_ = s.repo.UpdateScrumCardFlowMetrics",
		"payload, _ := json.Marshal",
		"raw, _ := json.Marshal(metrics)",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("Scrum flow metrics contains a swallowed failure %q", forbidden)
		}
	}
}

func TestScrumCardLLMEnqueueIsAtomicAndHasNoDuplicateFallback(t *testing.T) {
	source := readAPISource(t, "scrum_card_llm_enqueue.go")
	if !strings.Contains(source, "s.repo.EnqueueScrumCardJob(") {
		t.Fatal("Scrum card LLM jobs must use the atomic repository operation")
	}
	for _, forbidden := range []string{
		"s.repo.EnqueueJob(",
		"s.repo.UpdateScrumCard(",
		"if jobID, err := parseJobID(existing); err == nil",
		"if details, err := s.repo.GetJobDetails(ctx, jobID); err == nil",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("Scrum card LLM enqueue contains a non-atomic or swallowed path %q", forbidden)
		}
	}
}

func TestContextTelemetryDoesNotSilentlyDropPersistenceFailures(t *testing.T) {
	for _, path := range []string{"llm_context_telemetry.go", "scrum_pilot_metrics.go"} {
		source := readAPISource(t, path)
		if strings.Contains(source, "_ = s.repo.Record") {
			t.Errorf("%s silently drops telemetry persistence failures", path)
		}
	}
}

func TestOperationalStatusEndpointsDoNotReturnPartialSuccess(t *testing.T) {
	checks := map[string][]string{
		"activity.go":             {"llmActivity, _ :="},
		"data_sources_service.go": {"updated, _ := s.repo.UpdateDataSourceTestResult"},
		"network_settings.go":     {"if stored, err := s.repo.GetCoreURL", "writeEnvFile("},
	}
	for path, forbidden := range checks {
		source := readAPISource(t, path)
		for _, snippet := range forbidden {
			if strings.Contains(source, snippet) {
				t.Errorf("%s contains partial-success path %q", path, snippet)
			}
		}
	}
}

func TestScrumCardDatabaseJSONNeverSilentlyNormalizesCorruption(t *testing.T) {
	source := readAPISource(t, "project_handlers.go")
	for _, forbidden := range []string{
		"_ = json.Unmarshal(card.",
		"checklist, _ := json.Marshal",
		"refFiles, _ := json.Marshal",
		"chat, _ := json.Marshal",
		"tags, _ := json.Marshal",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("Scrum card conversion silently normalizes invalid state %q", forbidden)
		}
	}
}

func readAPISource(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func sourceBetween(t *testing.T, source, startMarker, endMarker string) string {
	t.Helper()
	start := strings.Index(source, startMarker)
	if start < 0 {
		t.Fatalf("missing source marker %q", startMarker)
	}
	end := strings.Index(source[start:], endMarker)
	if end < 0 {
		t.Fatalf("missing source marker %q", endMarker)
	}
	return source[start : start+end]
}

type autoWorkErrString string

func (e autoWorkErrString) Error() string {
	return string(e)
}
