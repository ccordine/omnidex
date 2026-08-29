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
		raw, opening.SemanticUncertaintyContractSHA256,
		opening.RendererVersion, opening.WorkKind,
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

func TestStationGapSemanticUncertaintyUsesRendererVersionedApplicationIntentAuthority(t *testing.T) {
	t.Parallel()
	for _, kind := range []assemblyline.WorkKind{
		assemblyline.WorkApplicationProductContext,
		assemblyline.WorkApplicationRequirementCoverage,
		assemblyline.WorkApplicationRequirement,
		assemblyline.WorkApplicationProjectStackConstraint,
	} {
		current := StationGapOpening{
			RendererVersion: assemblyline.PortableRendererV8,
			WorkKind:        string(kind),
		}
		contract, err := assemblyline.SemanticUncertaintyContractForPortableRenderer(
			current.RendererVersion, kind,
		)
		if err != nil {
			t.Fatal(err)
		}
		digest, err := contract.Digest()
		if err != nil {
			t.Fatal(err)
		}
		current.SemanticUncertaintyContract = contract
		current.SemanticUncertaintyContractSHA256 = digest
		if err := ValidateStationGapSemanticUncertainty(current); err != nil {
			t.Fatalf("current %s contract: %v", kind, err)
		}
		v7 := current
		v7.RendererVersion = assemblyline.HistoricalPortableRendererV7
		v7.SemanticUncertaintyContract, err =
			assemblyline.SemanticUncertaintyContractForPortableRenderer(
				v7.RendererVersion, kind,
			)
		if err != nil {
			t.Fatal(err)
		}
		v7.SemanticUncertaintyContractSHA256, err = v7.SemanticUncertaintyContract.Digest()
		if err != nil {
			t.Fatal(err)
		}
		if err := ValidateStationGapSemanticUncertainty(v7); err != nil {
			t.Fatalf("historical V7 %s contract: %v", kind, err)
		}

		for _, renderer := range []string{
			assemblyline.HistoricalPortableRendererV5,
			assemblyline.HistoricalPortableRendererV6,
		} {
			historical := current
			historical.RendererVersion = renderer
			historical.SemanticUncertaintyContract, err =
				assemblyline.SemanticUncertaintyContractForPortableRenderer(renderer, kind)
			if err != nil {
				t.Fatal(err)
			}
			historical.SemanticUncertaintyContractSHA256, err =
				historical.SemanticUncertaintyContract.Digest()
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateStationGapSemanticUncertainty(historical); err != nil {
				t.Fatalf("historical %s/%s contract: %v", renderer, kind, err)
			}
			historical.SemanticUncertaintyContract = current.SemanticUncertaintyContract
			historical.SemanticUncertaintyContractSHA256 = current.SemanticUncertaintyContractSHA256
			if err := ValidateStationGapSemanticUncertainty(historical); err == nil {
				t.Fatalf("historical %s/%s accepted current V2 authority", renderer, kind)
			}
		}
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
		raw, opening.SemanticUncertaintyContractSHA256,
		opening.RendererVersion, opening.WorkKind,
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
