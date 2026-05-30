package api

import "testing"

func TestParseScrumAutoReviewVerdictJSON(t *testing.T) {
	raw := `{"ready_for_review":false,"summary":"missing tests","gaps":["no unit tests"]}`
	got, ok := parseScrumAutoReviewVerdict(raw)
	if !ok || got.Ready || got.Summary != "missing tests" || len(got.Gaps) != 1 {
		t.Fatalf("got %#v ok=%v", got, ok)
	}
}

func TestParseScrumAutoReviewVerdictStatusLine(t *testing.T) {
	got, ok := parseScrumAutoReviewVerdict("SCRUM_REVIEW: ready\n")
	if !ok || !got.Ready {
		t.Fatalf("got %#v", got)
	}
	got, ok = parseScrumAutoReviewVerdict("SCRUM_REVIEW: not_ready")
	if !ok || got.Ready {
		t.Fatalf("got %#v", got)
	}
}

func TestLoadScrumAutoReviewConfig(t *testing.T) {
	settings := []byte(`{"scrum_auto_review":{"enabled":true,"bounce_column":"in_progress"}}`)
	cfg := loadScrumAutoReviewConfig(settings)
	if !cfg.Enabled || cfg.BounceColumn != "in_progress" {
		t.Fatalf("cfg=%#v", cfg)
	}
	cfg = loadScrumAutoReviewConfig([]byte(`{"scrum_auto_review":{"enabled":true,"bounce_column":"done"}}`))
	if cfg.BounceColumn != "assigned" {
		t.Fatalf("invalid bounce should default to assigned, got %q", cfg.BounceColumn)
	}
}

func TestIsScrumAutoReviewJob(t *testing.T) {
	if !isScrumAutoReviewJob([]byte(`{"scrum_auto_review":true}`)) {
		t.Fatal("expected auto review job")
	}
	if isScrumAutoReviewJob([]byte(`{"source":"omni-scrum"}`)) {
		t.Fatal("play job should not be auto review")
	}
}
