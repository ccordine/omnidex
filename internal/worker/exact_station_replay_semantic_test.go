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
	job, err := assemblyline.NewKnownArtifactTruthJob(assemblyline.KnownArtifactTruthInput{
		RequirementQuote: "The known semantic artifact must be absent.",
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := replayExactStationArtifact(
		job,
		`{"truth":"known_artifact_must_be_absent"}`,
	); err == nil || !strings.Contains(err.Error(), string(job.Kind)) {
		t.Fatalf("structured wrapper replay error=%v", err)
	}

	artifact, err := replayExactStationArtifact(job, string(assemblyline.KnownArtifactMustBeAbsent))
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Kind != string(job.Kind) || artifact.Source != string(assemblyline.KnownArtifactMustBeAbsent) {
		t.Fatalf("semantic artifact=%+v", artifact)
	}
}

func TestExactStationReplayValidatesCorrectionWithOriginalSemanticDecoder(t *testing.T) {
	t.Parallel()
	original, err := assemblyline.NewKnownArtifactTruthJob(assemblyline.KnownArtifactTruthInput{
		RequirementQuote: "The known semantic artifact must be absent.",
	})
	if err != nil {
		t.Fatal(err)
	}
	const invalid = `{"truth":"known_artifact_must_be_absent"}`
	job, err := assemblyline.NewRetainedResponseCorrectionJob(
		original,
		"Return only the registered raw truth value.",
		invalid,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := replayExactStationArtifact(job, invalid); err == nil ||
		!strings.Contains(err.Error(), "validate corrected known_artifact_truth leaf") {
		t.Fatalf("correction replay error=%v", err)
	}
	if _, err := replayExactStationArtifact(
		job,
		string(assemblyline.KnownArtifactMustBeAbsent),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := replayExactStationArtifact(
		job,
		" "+string(assemblyline.KnownArtifactMustBeAbsent),
	); err == nil || !strings.Contains(err.Error(), "preserve one exact trimmed leaf") {
		t.Fatalf("whitespace correction replay error=%v", err)
	}
}

func exactStationReplayUsesSpecializedProjection(kind assemblyline.WorkKind) bool {
	switch kind {
	case assemblyline.WorkApplicationTargetTree,
		assemblyline.WorkTypeScriptRepairGuidance,
		assemblyline.WorkFragmentGeneration,
		assemblyline.WorkFragmentModification,
		assemblyline.WorkFragmentCorrection:
		return true
	default:
		return false
	}
}
