package api

import (
	"context"
	"os"
	"strings"
	"testing"
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

func TestScrumRuntimeRetainsNoDeadBoardOrPathPlayHelpers(t *testing.T) {
	for path, forbidden := range map[string][]string{
		"scrum_play_queue.go":      {"startScrumCardPlay(", "pauseScrumCardPlay("},
		"scrum_service.go":         {"loadScrumContext(", "scrumGetCard(", "scrumProjectDirectory("},
		"scrum_channel_handler.go": {"scrumGetCard(", "scrumBoardMetadataFromProject("},
	} {
		source := readAPISource(t, path)
		for _, fragment := range forbidden {
			if strings.Contains(source, fragment) {
				t.Errorf("%s retains dead board/path play helper %q", path, fragment)
			}
		}
	}
}

func TestAutoWorkKickoffUsesOneLockedRepositoryTransition(t *testing.T) {
	source := readAPISource(t, "scrum_auto_play.go")
	required := []string{"s.repo.ApplyProjectAutoWork", "queue.ProjectAutoWorkPlay"}
	for _, snippet := range required {
		if !strings.Contains(source, snippet) {
			t.Fatalf("auto-work must use the locked repository transition; missing %q", snippet)
		}
	}
	forbidden := []string{
		"prepareScrumCardForAutoWork", "markScrumAutoWorkStartFailure", "startScrumCardPlay(",
	}
	for _, snippet := range forbidden {
		if strings.Contains(source, snippet) {
			t.Fatalf("auto-work retains a superseded multi-transaction start path %q", snippet)
		}
	}
}

func TestGlobalAutoWorkDoesNotSkipAuthoritativeFailures(t *testing.T) {
	source := readAPISource(t, "scrum_global_autowork.go")
	for _, forbidden := range []string{
		"scrum global auto-work load project=%d",
		"scrum global running reconcile project=%d",
		"if board, err := s.scrumBoardMetadataFromProject(ctx, candidate.projectID); err == nil",
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

func TestScrumRuntimeHasNoBroadCardWriterOrModelVisibleRepositoryIdentity(t *testing.T) {
	t.Parallel()
	checks := map[string][]string{
		"../queue/scrum_card_mutation.go":        {"UpdateScrumCard(", "UpdateScrumCardWithMessages(", "map[string]any"},
		"../queue/step_attempt_domain_writes.go": {"UpdateScrumCardByStepAttempt("},
		"scrum_channel_handler.go":               {"scrumGetCard(", "ProjectDirectory", "project_directory"},
		"scrum_channel_operation.go":             {"Project directory:", "RefFiles", "recipe_id", "agent_config"},
	}
	for path, forbidden := range checks {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, snippet := range forbidden {
			if strings.Contains(string(raw), snippet) {
				t.Errorf("%s retains forbidden Scrum runtime authority %q", path, snippet)
			}
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
	source := readAPISource(t, "scrum_card_conversion.go")
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
