package worker

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func applicationRequirementCandidateResultRelationAuthorityForTest(
	t testing.TB,
	candidate string,
) assemblyline.ApplicationRequirementCandidateResultRelationInput {
	t.Helper()
	kind := applicationRequirementCandidateKindReceiptForTest(
		t,
		candidate,
		assemblyline.ApplicationRequirementCandidateTaskLocal,
	)
	cardinality, err := assemblyline.DecodeApplicationRequirementCandidateCardinalityResult(
		assemblyline.ApplicationRequirementCandidateCardinalityInput{Candidate: candidate},
		assemblyline.ApplicationRequirementOneRuntimeOutcome,
	)
	if err != nil {
		t.Fatal(err)
	}
	return assemblyline.ApplicationRequirementCandidateResultRelationInput{
		Candidate: candidate, Kind: kind, Cardinality: cardinality,
	}
}

func applicationRequirementCandidateResultRelationReceiptForTest(
	t testing.TB,
	input assemblyline.ApplicationRequirementCandidateResultRelationInput,
	relation string,
) assemblyline.ApplicationRequirementCandidateResultRelationResult {
	t.Helper()
	derivedPresence := assemblyline.ApplicationRequirementCandidateResultPresent
	if relation == assemblyline.ApplicationRequirementNoDerivedResult {
		derivedPresence = assemblyline.ApplicationRequirementCandidateResultAbsent
	}
	derivedInput := assemblyline.ApplicationRequirementCandidateResultPresenceInput{
		Candidate: input.Candidate, Kind: input.Kind, Cardinality: input.Cardinality,
		Dimension: assemblyline.ApplicationRequirementDerivedValueDimension,
	}
	derived, err := assemblyline.DecodeApplicationRequirementCandidateResultPresenceResult(
		derivedInput,
		string(derivedPresence),
	)
	if err != nil {
		t.Fatal(err)
	}
	if derivedPresence == assemblyline.ApplicationRequirementCandidateResultAbsent {
		result, err := assemblyline.ResolveApplicationRequirementCandidateResultRelation(
			input,
			derived,
			nil,
		)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}

	determiningPresence := assemblyline.ApplicationRequirementCandidateResultAbsent
	if relation == assemblyline.ApplicationRequirementExplicitResultRelation {
		determiningPresence = assemblyline.ApplicationRequirementCandidateResultPresent
	} else if relation != assemblyline.ApplicationRequirementMissingResultRelation {
		t.Fatalf("unregistered result-relation fixture %q", relation)
	}
	determiningInput := assemblyline.ApplicationRequirementCandidateResultPresenceInput{
		Candidate: input.Candidate, Kind: input.Kind, Cardinality: input.Cardinality,
		Dimension:            assemblyline.ApplicationRequirementDeterminingRelationDimension,
		DerivedValuePresence: &derived,
	}
	determining, err := assemblyline.DecodeApplicationRequirementCandidateResultPresenceResult(
		determiningInput,
		string(determiningPresence),
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := assemblyline.ResolveApplicationRequirementCandidateResultRelation(
		input,
		derived,
		&determining,
	)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func applicationRequirementCandidateResultPresenceInputForTest(
	job assemblyline.PortableJob,
) (assemblyline.ApplicationRequirementCandidateResultPresenceInput, error) {
	var input assemblyline.ApplicationRequirementCandidateResultPresenceInput
	if job.Kind != assemblyline.WorkApplicationRequirementCandidateResultRelation {
		return input, fmt.Errorf("result-presence fixture received work kind %q", job.Kind)
	}
	if err := json.Unmarshal(job.Payload, &input); err != nil {
		return input, err
	}
	if _, err := assemblyline.NewApplicationRequirementCandidateResultPresenceJob(input); err != nil {
		return input, err
	}
	return input, nil
}

func applicationRequirementCandidateResultPresenceForRelationForTest(
	job assemblyline.PortableJob,
	relation string,
) (string, error) {
	input, err := applicationRequirementCandidateResultPresenceInputForTest(job)
	if err != nil {
		return "", err
	}
	switch input.Dimension {
	case assemblyline.ApplicationRequirementDerivedValueDimension:
		switch relation {
		case assemblyline.ApplicationRequirementNoDerivedResult:
			return string(assemblyline.ApplicationRequirementCandidateResultAbsent), nil
		case assemblyline.ApplicationRequirementExplicitResultRelation,
			assemblyline.ApplicationRequirementMissingResultRelation:
			return string(assemblyline.ApplicationRequirementCandidateResultPresent), nil
		}
	case assemblyline.ApplicationRequirementDeterminingRelationDimension:
		switch relation {
		case assemblyline.ApplicationRequirementExplicitResultRelation:
			return string(assemblyline.ApplicationRequirementCandidateResultPresent), nil
		case assemblyline.ApplicationRequirementMissingResultRelation:
			return string(assemblyline.ApplicationRequirementCandidateResultAbsent), nil
		}
	default:
		return "", fmt.Errorf("unregistered result-presence dimension %q", input.Dimension)
	}
	return "", fmt.Errorf(
		"result relation %q cannot answer dimension %q",
		relation,
		input.Dimension,
	)
}
