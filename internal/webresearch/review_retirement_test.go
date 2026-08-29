package webresearch

import (
	"os"
	"strings"
	"testing"
)

func TestWebReviewAndSynthesisCorrectionSourcesAreRetired(t *testing.T) {
	for _, path := range []string{
		"correction.go",
		"portable_correction.go",
		"portable_review.go",
		"review.go",
		"../assemblyline/web_claim_evidence_review.go",
		"../assemblyline/web_grounded_synthesis_correction.go",
		"../assemblyline/web_review_claim_leaves.go",
		"../assemblyline/web_review_claim_verdict.go",
		"../assemblyline/web_review_issue_detail.go",
		"../assemblyline/web_review_issue_evidence_relation.go",
		"../worker/objective_web_identity.go",
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("retired web review or correction source %s still exists or cannot be checked: %v", path, err)
		}
	}
}

func TestWebProductionFlowHasNoReviewOrCorrectionAuthority(t *testing.T) {
	production := []string{
		"types.go",
		"stations.go",
		"machine.go",
		"completion.go",
		"../assemblyline/portable_job_registry.go",
		"../assemblyline/portable_job_payload.go",
		"../assemblyline/portable_job_render_database_web.go",
		"../assemblyline/portable_response_maximum_web.go",
		"../worker/objective_web_workflow.go",
		"../worker/objective_turn_runtime.go",
		"../worker/exact_station_replay_web_semantic.go",
		"../queue/station_gap_mapping.go",
		"../station/id.go",
		"../config/station_models.go",
		"../modelconfig/config.go",
		"../modelconfig/routing.go",
	}
	forbidden := []string{
		"WebGroundedSynthesisCorrection",
		"WebClaimEvidenceReview",
		"GroundedSynthesisCorrectionStation",
		"ClaimEvidenceReviewStation",
		"WorkWebReviewClaim",
		"WorkWebReviewIssue",
		"webModelIdentityGuard",
		"validWebReviewCallLedger",
		"SynthesisCorrectionZeroDeltas",
		"ClaimEvidenceReviewCalls",
		"AcceptancePredicate",
	}
	for _, path := range production {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read production source %s: %v", path, err)
		}
		for _, token := range forbidden {
			if strings.Contains(string(raw), token) {
				t.Fatalf("production source %s retains retired authority %q", path, token)
			}
		}
	}
}
