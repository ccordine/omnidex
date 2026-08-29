package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
)

func TestStationGapSemanticUncertaintyRoundTripIsExactAndNotModelVisible(t *testing.T) {
	t.Parallel()
	opening := validatedStationGapUncertaintyFixture(t)
	raw, err := canonicalStationGapSemanticUncertainty(opening)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeStationGapSemanticUncertainty(
		raw, opening.SemanticUncertaintyContractSHA256, opening.WorkKind,
	)
	if err != nil {
		t.Fatal(err)
	}
	if decoded != opening.SemanticUncertaintyContract {
		t.Fatalf("decoded contract=%+v want %+v", decoded, opening.SemanticUncertaintyContract)
	}
	for _, modelVisible := range []string{opening.Prompt, opening.ProjectionEnvelope} {
		if strings.Contains(modelVisible, opening.SemanticUncertaintyContract.ID) ||
			strings.Contains(modelVisible, opening.SemanticUncertaintyContractSHA256) ||
			strings.Contains(modelVisible, opening.SemanticUncertaintyContract.DeterministicLimitation) {
			t.Fatalf("model-visible station projection contains uncertainty evidence: %q", modelVisible)
		}
	}
}

func TestStationGapSemanticUncertaintyRejectsEveryAuthorityMismatch(t *testing.T) {
	t.Parallel()
	opening := validatedStationGapUncertaintyFixture(t)
	tests := map[string]func(*StationGapOpening){
		"work kind": func(candidate *StationGapOpening) {
			candidate.WorkKind = string(assemblyline.WorkGroundedAnswerText)
		},
		"contract work kind": func(candidate *StationGapOpening) {
			candidate.SemanticUncertaintyContract.WorkKind = assemblyline.WorkGroundedAnswerText
		},
		"contract id": func(candidate *StationGapOpening) {
			candidate.SemanticUncertaintyContract.ID += ".forged"
		},
		"contract field": func(candidate *StationGapOpening) {
			candidate.SemanticUncertaintyContract.ExactQuestion = "Which other value?"
		},
		"digest": func(candidate *StationGapOpening) {
			candidate.SemanticUncertaintyContractSHA256 = strings.Repeat("0", 64)
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := opening
			mutate(&candidate)
			if err := ValidateStationGapSemanticUncertainty(candidate); err == nil {
				t.Fatalf("accepted forged %s authority", name)
			}
		})
	}
}

func TestStationGapSemanticUncertaintyDecodeRejectsUnknownJSONField(t *testing.T) {
	t.Parallel()
	opening := validatedStationGapUncertaintyFixture(t)
	raw, err := canonicalStationGapSemanticUncertainty(opening)
	if err != nil {
		t.Fatal(err)
	}
	raw = append(append([]byte{}, raw[:len(raw)-1]...), []byte(`,"extra":"forbidden"}`)...)
	if _, err := decodeStationGapSemanticUncertainty(
		raw, opening.SemanticUncertaintyContractSHA256, opening.WorkKind,
	); err == nil {
		t.Fatal("semantic uncertainty decoder accepted an unknown JSON field")
	}
}

func validatedStationGapUncertaintyFixture(t *testing.T) StationGapOpening {
	t.Helper()
	record := stationGapOpenFixture(t, model.StepAttemptAuthority{
		JobID: 1, Generation: 1, StepID: 1, Attempt: 1, WorkerID: "worker",
	})
	opening, err := validateStationGapOpening(record)
	if err != nil {
		t.Fatal(err)
	}
	return opening
}
