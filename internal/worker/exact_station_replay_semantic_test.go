package worker

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestExactStationReplayRejectsInvalidRawForEveryRegisteredSemanticLeaf(t *testing.T) {
	t.Parallel()
	for _, kind := range assemblyline.AllWorkKinds() {
		kind := kind
		if exactStationReplayUsesSpecializedProjection(kind) {
			continue
		}
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()
			job := assemblyline.PortableJob{Kind: kind, Payload: json.RawMessage(`{}`)}
			artifact, handled, err := replayExactStationSemanticArtifact(
				job,
				"",
				ExactStationReplayArtifact{Kind: "exact_final_response"},
			)
			if !handled {
				t.Fatalf("registered semantic leaf %q has no exact replay decoder", kind)
			}
			if err == nil {
				t.Fatalf("registered semantic leaf %q accepted invalid raw output", kind)
			}
			if artifact.Kind != string(kind) {
				t.Fatalf("artifact kind=%q, want %q", artifact.Kind, kind)
			}
		})
	}
}

func TestExactStationReplayUsesSemanticDecoderInsteadOfRawFallback(t *testing.T) {
	t.Parallel()
	absence, err := assemblyline.NewRepositoryArtifactAbsenceJob(assemblyline.RepositoryArtifactAbsenceInput{
		RequirementQuote: "The known semantic artifact must be absent.",
	})
	if err != nil {
		t.Fatal(err)
	}
	creation, err := assemblyline.NewPlainTextArtifactCreationJob(
		assemblyline.PlainTextArtifactCreationInput{
			RequirementQuote: "Create ARTIFACT_1 containing the complete note: Release ready.",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		job       assemblyline.PortableJob
		candidate string
		wrapped   string
	}{
		{
			job: absence, candidate: string(assemblyline.RepositoryArtifactMustBeAbsent),
			wrapped: `{"relation":"repository_artifact_must_be_absent"}`,
		},
		{
			job: creation, candidate: string(assemblyline.OneNewCompletePlainTextArtifactRequired),
			wrapped: `{"relation":"one_new_complete_plain_text_artifact_required"}`,
		},
	} {
		if _, err := replayExactStationArtifact(fixture.job, fixture.wrapped); err == nil ||
			!strings.Contains(err.Error(), string(fixture.job.Kind)) {
			t.Fatalf("%s structured wrapper replay error=%v", fixture.job.Kind, err)
		}
		artifact, err := replayExactStationArtifact(fixture.job, fixture.candidate)
		if err != nil {
			t.Fatal(err)
		}
		if artifact.Kind != string(fixture.job.Kind) || artifact.Source != fixture.candidate {
			t.Fatalf("semantic artifact=%+v", artifact)
		}
	}
}

func TestExactStationReplayPreservesCurrentRequirementRefinementKinds(t *testing.T) {
	t.Parallel()
	kindJob, err := assemblyline.NewApplicationRequirementCandidateKindJob(
		assemblyline.ApplicationRequirementCandidateKindInput{
			Candidate: "Display the current status.",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	kindArtifact, err := replayExactStationArtifact(
		kindJob, assemblyline.ApplicationRequirementCandidateTaskLocal,
	)
	if err != nil || kindArtifact.Kind != string(kindJob.Kind) {
		t.Fatalf("candidate-kind artifact=%+v error=%v", kindArtifact, err)
	}

	const resultCandidate = "Display the current status."
	resultKindInput := assemblyline.ApplicationRequirementCandidateKindInput{
		Candidate: resultCandidate,
	}
	resultKind, err := assemblyline.DecodeApplicationRequirementCandidateKindResult(
		resultKindInput, assemblyline.ApplicationRequirementCandidateTaskLocal,
	)
	if err != nil {
		t.Fatal(err)
	}
	resultCardinalityInput := assemblyline.ApplicationRequirementCandidateCardinalityInput{
		Candidate: resultCandidate,
	}
	resultCardinality, err := assemblyline.DecodeApplicationRequirementCandidateCardinalityResult(
		resultCardinalityInput, assemblyline.ApplicationRequirementOneRuntimeOutcome,
	)
	if err != nil {
		t.Fatal(err)
	}
	resultAuthority := assemblyline.ApplicationRequirementCandidateResultRelationInput{
		Candidate: resultCandidate, Kind: resultKind, Cardinality: resultCardinality,
	}
	resultJob, err := assemblyline.NewApplicationRequirementCandidateResultRelationJob(
		resultAuthority,
	)
	if err != nil {
		t.Fatal(err)
	}
	resultArtifact, err := replayExactStationArtifact(
		resultJob, assemblyline.ApplicationRequirementNoDerivedResult,
	)
	if err != nil || resultArtifact.Kind != string(resultJob.Kind) {
		t.Fatalf("result-relation artifact=%+v error=%v", resultArtifact, err)
	}

	request := "Create a browser status board that displays one current status and offers refresh."
	applicationContext, err := assemblyline.BootstrapApplicationContext(
		request, assemblyline.ApplicationWorkspaceEmpty,
	)
	if err != nil {
		t.Fatal(err)
	}
	coverageInput := assemblyline.ApplicationRequirementCoverageInput{
		UserRequest: request, Context: applicationContext,
		AcceptedRequirements: []string{"Display the current status."},
		ExcludedCandidates:   []string{},
	}
	coverage, err := assemblyline.DecodeApplicationRequirementCoverageLeaf(
		coverageInput, assemblyline.ApplicationRequirementRemains,
	)
	if err != nil {
		t.Fatal(err)
	}
	duplicateJob, err := assemblyline.NewApplicationRequirementCandidateDuplicateReplacementJob(
		assemblyline.ApplicationRequirementCandidateDuplicateReplacementInput{
			GenerationAuthority: assemblyline.ApplicationRequirementCandidateInput{
				Authority: coverageInput, Coverage: coverage,
			},
			CurrentCandidate: coverageInput.AcceptedRequirements[0],
			Duplicate: assemblyline.ApplicationRequirementCandidateDuplicateIdentity{
				Set: assemblyline.ApplicationRequirementDuplicateAcceptedRequirement, Index: 0,
			},
			Defect: assemblyline.ApplicationRequirementDuplicateCandidateDefect,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	duplicateArtifact, err := replayExactStationArtifact(
		duplicateJob, "Offer a refresh control.",
	)
	if err != nil || duplicateArtifact.Kind != string(duplicateJob.Kind) {
		t.Fatalf("duplicate-replacement artifact=%+v error=%v", duplicateArtifact, err)
	}

	const vague = "Display a correct refreshed result."
	vagueKind, err := assemblyline.DecodeApplicationRequirementCandidateKindResult(
		assemblyline.ApplicationRequirementCandidateKindInput{Candidate: vague},
		assemblyline.ApplicationRequirementCandidateTaskLocal,
	)
	if err != nil {
		t.Fatal(err)
	}
	vagueCardinality, err := assemblyline.DecodeApplicationRequirementCandidateCardinalityResult(
		assemblyline.ApplicationRequirementCandidateCardinalityInput{Candidate: vague},
		assemblyline.ApplicationRequirementOneRuntimeOutcome,
	)
	if err != nil {
		t.Fatal(err)
	}
	vagueAuthority := assemblyline.ApplicationRequirementCandidateResultRelationInput{
		Candidate: vague, Kind: vagueKind, Cardinality: vagueCardinality,
	}
	vagueRelation, err := assemblyline.DecodeApplicationRequirementCandidateResultRelationResult(
		vagueAuthority, assemblyline.ApplicationRequirementMissingResultRelation,
	)
	if err != nil {
		t.Fatal(err)
	}
	correctionJob, err := assemblyline.NewApplicationRequirementCandidateResultRelationCorrectionJob(
		assemblyline.ApplicationRequirementCandidateResultRelationCorrectionInput{
			GenerationAuthority: assemblyline.ApplicationRequirementCandidateInput{
				Authority: coverageInput, Coverage: coverage,
			},
			CandidateAuthority: vagueAuthority,
			ResultRelation:     vagueRelation,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	correctionArtifact, err := replayExactStationArtifact(
		correctionJob, "Refresh the displayed status from its current runtime value.",
	)
	if err != nil || correctionArtifact.Kind != string(correctionJob.Kind) {
		t.Fatalf("result-relation correction artifact=%+v error=%v", correctionArtifact, err)
	}
}

func exactStationReplayUsesSpecializedProjection(kind assemblyline.WorkKind) bool {
	switch kind {
	case assemblyline.WorkApplicationTargetTree,
		assemblyline.WorkTypeScriptRepairGuidance,
		assemblyline.WorkFragmentGeneration,
		assemblyline.WorkFragmentGenerationReplacement,
		assemblyline.WorkFragmentModification,
		assemblyline.WorkFragmentCorrection:
		return true
	default:
		return false
	}
}
