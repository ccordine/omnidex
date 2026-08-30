package assemblyline

import (
	"os"
	"strings"
	"testing"
)

func TestWebSynthesisCoverageGenerationFixedPointIsAbsent(t *testing.T) {
	if _, err := os.Stat("web_grounded_synthesis_leaves.go"); !os.IsNotExist(err) {
		t.Fatalf("retired web synthesis fixed-point source still exists or cannot be checked: %v", err)
	}
	production := []string{
		"portable_job_payload.go",
		"portable_job_registry.go",
		"portable_job_render_database_web.go",
		"portable_response_framing.go",
		"portable_response_maximum_web.go",
		"semantic_uncertainty_web.go",
		"../queue/station_gap_mapping.go",
		"../webresearch/portable_synthesis.go",
		"../worker/exact_station_replay_web_semantic.go",
		"../worker/objective_candidate_path_boundary.go",
		"../../database/setup.sql",
		"../../docs/CHARMELEON-CHAT-WEBSEARCH-INVARIANTS.md",
	}
	forbidden := []string{
		"WorkWebSynthesisParagraphCoverage",
		"WebSynthesisParagraphLeafInput",
		"WebSynthesisParagraphCoverageDecision",
		"WebSynthesisParagraphDecision",
		"WebSynthesisParagraphRemains",
		"WebSynthesisNoUncoveredParagraph",
		"NewWebSynthesisParagraphCoverageJob",
		"NewWebSynthesisParagraphJob",
		"DecodeWebSynthesisParagraphCoverageDecision",
		"DecodeWebSynthesisParagraphDecision",
		`"web_synthesis_paragraph_coverage"`,
		`"web_synthesis_paragraph"`,
		`'web_synthesis_paragraph_coverage'`,
		`'web_synthesis_paragraph'`,
	}
	for _, path := range production {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read production source %s: %v", path, err)
		}
		for _, token := range forbidden {
			if strings.Contains(string(raw), token) {
				t.Fatalf("production source %s retains fixed-point authority %q", path, token)
			}
		}
	}
}
