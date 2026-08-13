package worker

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
)

func TestDesiredRepositoryCallProofUsesExactWorkKinds(t *testing.T) {
	t.Parallel()
	original, err := assemblyline.NewRequirementPartitionJob(assemblyline.RequirementPartitionInput{
		SourceText: "Add one value.", Mode: assemblyline.RequirementExtractFeatures,
	})
	if err != nil {
		t.Fatal(err)
	}
	correctionPayload, err := json.Marshal(assemblyline.ResponseCorrectionInput{
		Original:          original,
		ValidationFailure: "one field is invalid",
	})
	if err != nil {
		t.Fatal(err)
	}
	proof, err := compileDesiredRepositoryCallProof([]queue.StationAttemptCallEvidence{
		{OpeningID: 1, WorkKind: assemblyline.WorkConversationObjectiveKind, Payload: `{}`, Prompt: "classify"},
		{OpeningID: 2, WorkKind: assemblyline.WorkResponseCorrection, Payload: string(correctionPayload), Prompt: "correct"},
		{OpeningID: 3, WorkKind: assemblyline.WorkFragmentGeneration, Payload: `{}`, Prompt: "generate", Response: "func Added() int { return 1 }"},
		{OpeningID: 4, WorkKind: assemblyline.WorkFragmentCorrection, Payload: `{}`, Prompt: "repair", Response: "func Added() int { return 2 }"},
	}, []string{"omni_added_artifact.go"})
	if err != nil {
		t.Fatal(err)
	}
	if proof.TotalModelCalls != 4 || proof.SemanticGapCalls != 2 ||
		proof.DeclarationGenerationCalls != 1 || proof.DeclarationCorrectionCalls != 1 ||
		proof.ModelVisibleTargetPaths != 0 || proof.ModelSelectedMutationOperations != 0 {
		t.Fatalf("proof=%+v", proof)
	}
}

func TestDesiredRepositoryCallProofCountsGenericFragmentCorrectionAsCorrection(t *testing.T) {
	t.Parallel()
	original, err := assemblyline.NewFragmentGenerationJob(assemblyline.FragmentGenerationInput{
		Language: "go", Signature: "func Added() int",
		Behavior: "Return the accepted integer.",
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(assemblyline.ResponseCorrectionInput{
		Original: original, ValidationFailure: "candidate did not match the signature",
	})
	if err != nil {
		t.Fatal(err)
	}
	proof, err := compileDesiredRepositoryCallProof([]queue.StationAttemptCallEvidence{
		{OpeningID: 1, WorkKind: assemblyline.WorkFragmentGeneration, Payload: string(original.Payload)},
		{OpeningID: 2, WorkKind: assemblyline.WorkResponseCorrection, Payload: string(payload)},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if proof.DeclarationGenerationCalls != 1 || proof.DeclarationCorrectionCalls != 1 {
		t.Fatalf("fragment call proof=%+v", proof)
	}
}

func TestDesiredRepositoryCallProofRejectsVisibleTargetBeforeMutation(t *testing.T) {
	t.Parallel()
	_, err := compileDesiredRepositoryCallProof([]queue.StationAttemptCallEvidence{{
		OpeningID: 1, WorkKind: assemblyline.WorkConversationObjectiveKind,
		Prompt: "obsolete.go must no longer exist", Response: `{"kind":"workspace_mutation"}`,
	}}, []string{"obsolete.go"})
	if err == nil || !strings.Contains(err.Error(), "model-visible target path") {
		t.Fatalf("visible path error=%v", err)
	}
}

func TestDesiredRepositoryCallProofRejectsVisibleTargetBasename(t *testing.T) {
	t.Parallel()
	_, err := compileDesiredRepositoryCallProof([]queue.StationAttemptCallEvidence{{
		OpeningID: 1, WorkKind: assemblyline.WorkConversationObjectiveKind,
		Prompt: "obsolete.go must no longer exist", Response: `{"kind":"workspace_mutation"}`,
	}}, []string{"internal/legacy/obsolete.go"})
	if err == nil || !strings.Contains(err.Error(), "model-visible target path") {
		t.Fatalf("visible basename error=%v", err)
	}
}

func TestDesiredRepositoryCallProofRejectsModelSelectedPhysicalOperation(t *testing.T) {
	t.Parallel()
	_, err := compileDesiredRepositoryCallProof([]queue.StationAttemptCallEvidence{{
		OpeningID: 1, WorkKind: assemblyline.WorkFragmentGeneration,
		Prompt: "generate declaration", Response: `{"delete_file":"ARTIFACT_1"}`,
	}}, nil)
	if err == nil || !strings.Contains(err.Error(), "model-selected mutation operation") {
		t.Fatalf("physical operation error=%v", err)
	}
}
