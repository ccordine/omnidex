package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/evidence"
)

func TestDeterministicCodingPostProcessingNeedsNoModelJudgment(t *testing.T) {
	intent := deterministicCodingTestIntent()
	result := artifacts.SubtaskResultArtifact{
		SubtaskID: "coordinate_implementation", Kind: artifacts.SubtaskKindExecute,
		RoleID: "subtask_executor", ObjectiveID: "build", Objective: "Build the requested application",
		Priority: 100, RequiredCapabilities: []string{capabilityWorkspaceWrite, capabilityCommandExecute},
		Summary: "Completed deterministic coding workflow: planned_files=5 accepted_mutations=5 verification=go test ./...",
		Sources: []string{"workspace"},
	}

	analysis, handled, err := buildDeterministicV3CodingAnalysis(intent, []artifacts.SubtaskResultArtifact{result})
	if err != nil || !handled {
		t.Fatalf("analysis handled=%t err=%v", handled, err)
	}
	if analysis.Summary != result.Summary || len(analysis.Blockers) != 0 || len(analysis.Assumptions) != 0 {
		t.Fatalf("analysis=%+v", analysis)
	}

	draft, handled, err := buildDeterministicV3CodingResponse(intent, analysis, []artifacts.SubtaskResultArtifact{result})
	if err != nil || !handled {
		t.Fatalf("response handled=%t err=%v", handled, err)
	}
	if !strings.Contains(draft.Response, result.Summary) {
		t.Fatalf("response=%q", draft.Response)
	}

	records := []evidence.Record{
		{ID: 11, Kind: evidence.KindGeneratedDiff, Metadata: map[string]any{"mutation": true, "succeeded": true}},
		{ID: 12, Kind: evidence.KindTestResult, Command: "go test ./...", Metadata: map[string]any{"succeeded": true}},
	}
	verification, handled, err := buildDeterministicV3CodingVerification(intent, records)
	if err != nil || !handled {
		t.Fatalf("verification handled=%t err=%v", handled, err)
	}
	if verification.Verdict != artifacts.VerificationVerdictPass || !verification.IndependentChallenge {
		t.Fatalf("verification=%+v", verification)
	}
	if len(verification.ObjectiveCoverage) != 1 || !verification.ObjectiveCoverage[0].Satisfied {
		t.Fatalf("coverage=%+v", verification.ObjectiveCoverage)
	}
}

func TestDeterministicCodingVerificationFailsWithoutRequiredEvidence(t *testing.T) {
	intent := deterministicCodingTestIntent()
	_, handled, err := buildDeterministicV3CodingVerification(intent, []evidence.Record{
		{ID: 12, Kind: evidence.KindTestResult, Command: "go test ./...", Metadata: map[string]any{"succeeded": true}},
	})
	if !handled || err == nil || !strings.Contains(err.Error(), "generated-diff") {
		t.Fatalf("handled=%t err=%v", handled, err)
	}
}

func TestDeterministicCodingPostProcessingDeclinesNonCodingIntent(t *testing.T) {
	intent := artifacts.IntentArtifact{Objectives: []artifacts.Objective{{
		ID: "answer", Description: "Answer the question", Priority: 100,
	}}}
	if _, handled, err := buildDeterministicV3CodingAnalysis(intent, nil); err != nil || handled {
		t.Fatalf("analysis handled=%t err=%v", handled, err)
	}
	if _, handled, err := buildDeterministicV3CodingResponse(intent, artifacts.AnalysisArtifact{}, nil); err != nil || handled {
		t.Fatalf("response handled=%t err=%v", handled, err)
	}
	if _, handled, err := buildDeterministicV3CodingVerification(intent, nil); err != nil || handled {
		t.Fatalf("verification handled=%t err=%v", handled, err)
	}
}

func deterministicCodingTestIntent() artifacts.IntentArtifact {
	return artifacts.IntentArtifact{
		UserGoal: "Build the requested application", RequiresAction: true,
		RequiredCapabilities: []string{capabilityWorkspaceWrite, capabilityCommandExecute},
		CompletionCriteria:   []string{"the complete application passes its tests"},
		Objectives: []artifacts.Objective{{
			ID: "build", Description: "Build the requested application", Priority: 100, RequiresAction: true,
			RequiredCapabilities: []string{capabilityWorkspaceWrite, capabilityCommandExecute},
			AcceptanceCriteria:   []string{"the complete application passes its tests"},
		}},
	}
}
