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
