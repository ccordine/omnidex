package assemblyline

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestApplicationContextQuestionInventoryRetiresIterativeNeedControl(t *testing.T) {
	t.Parallel()
	for _, retiredFile := range []string{
		"application_context_need_leaves.go",
		"application_context_next_need.go",
	} {
		if _, err := os.Stat(retiredFile); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("retired application context source %s still exists or cannot be checked: %v", retiredFile, err)
		}
	}
	files := []string{
		"application_context.go",
		"application_context_question_inventory.go",
		"application_context_question_necessity.go",
		"application_context_question_relation.go",
		"portable_job_payload.go",
		"portable_job_registry.go",
		"portable_job_render_application.go",
		"portable_response_framing.go",
		"portable_response_maximum_application.go",
		"semantic_uncertainty_application.go",
		"../queue/station_gap_mapping.go",
		"../worker/exact_station_replay_application_semantic.go",
		"../worker/v3_application_context_resolution.go",
		"../../database/setup.sql",
		"../../docs/CHARMANDER_ASSEMBLY_LINE.md",
		"../../docs/CHARMELEON_COGNITION_RESOLUTION.md",
		"../../docs/CHARMELEON_CONTEXT_SYSTEM.md",
	}
	retired := []string{
		"application_context_need_coverage",
		"application_context_need_question",
		"application_context_next_need",
		"CONTEXT_NEED_REMAINS",
		"NO_UNCOVERED_CONTEXT_NEED",
		"NO_NECESSARY_REPOSITORY_FACT",
		"ApplicationContextNeedLeafInput",
		"ApplicationContextNeedDecision",
		"ApplicationContextNeedSchemaV1",
		"ApplicationContextNextNeedInput",
		"WorkApplicationContextNextNeed",
		"BuildApplicationContextNeedCoveragePrompt",
		"BuildApplicationContextNeedQuestionPrompt",
		"BuildApplicationContextNextNeedPrompt",
		"DecodeApplicationContextNeedCoverageLeaf",
		"DecodeApplicationContextNeedQuestionLeaf",
		"DecodeApplicationContextNextNeedLeaf",
		"AssembleApplicationContextNeedDecision",
		"MaxApplicationEvidenceNeeds",
		"AcceptedQuestions",
		"NECESSARY_DISTINCT_REPOSITORY_FACT",
	}
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, symbol := range retired {
			if strings.Contains(string(raw), symbol) {
				t.Fatalf("%s retains retired application-context symbol %q", file, symbol)
			}
		}
	}
}
