package worker

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func applicationRequirementCandidateContentPresenceInputForTest(
	job assemblyline.PortableJob,
) (assemblyline.ApplicationRequirementCandidateContentPresenceInput, error) {
	var input assemblyline.ApplicationRequirementCandidateContentPresenceInput
	if err := json.Unmarshal(job.Payload, &input); err != nil {
		return input, err
	}
	return input, nil
}

func applicationRequirementCandidateContentPresenceForKindForTest(
	job assemblyline.PortableJob,
	kind string,
) (string, error) {
	input, err := applicationRequirementCandidateContentPresenceInputForTest(job)
	if err != nil {
		return "", err
	}
	runtimePresent, nonRuntimePresent := false, false
	switch kind {
	case assemblyline.ApplicationRequirementCandidateTaskLocal:
		runtimePresent = true
	case assemblyline.ApplicationRequirementCandidateNonRuntime:
		nonRuntimePresent = true
	case assemblyline.ApplicationRequirementCandidateMixed:
		runtimePresent, nonRuntimePresent = true, true
	default:
		return "", fmt.Errorf("unregistered candidate kind %q", kind)
	}
	present := runtimePresent
	switch input.Dimension {
	case assemblyline.ApplicationRequirementCandidateRuntimeContentDimension:
	case assemblyline.ApplicationRequirementCandidateNonRuntimeContentDimension:
		present = nonRuntimePresent
	default:
		return "", fmt.Errorf("unregistered candidate content dimension %q", input.Dimension)
	}
	if present {
		return string(assemblyline.ApplicationRequirementCandidateContentPresent), nil
	}
	return string(assemblyline.ApplicationRequirementCandidateContentAbsent), nil
}

func applicationRequirementCandidateKindReceiptForTest(
	t testing.TB,
	candidate string,
	kind string,
) assemblyline.ApplicationRequirementCandidateKindResult {
	t.Helper()
	presence := func(
		dimension assemblyline.ApplicationRequirementCandidateContentDimension,
		value assemblyline.ApplicationRequirementCandidateContentPresence,
	) assemblyline.ApplicationRequirementCandidateContentPresenceResult {
		input := assemblyline.ApplicationRequirementCandidateContentPresenceInput{
			Candidate: candidate,
			Dimension: dimension,
		}
		result, err := assemblyline.DecodeApplicationRequirementCandidateContentPresenceResult(
			input,
			string(value),
		)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	runtimePresence, nonRuntimePresence :=
		assemblyline.ApplicationRequirementCandidateContentAbsent,
		assemblyline.ApplicationRequirementCandidateContentAbsent
	switch kind {
	case assemblyline.ApplicationRequirementCandidateTaskLocal:
		runtimePresence = assemblyline.ApplicationRequirementCandidateContentPresent
	case assemblyline.ApplicationRequirementCandidateNonRuntime:
		nonRuntimePresence = assemblyline.ApplicationRequirementCandidateContentPresent
	case assemblyline.ApplicationRequirementCandidateMixed:
		runtimePresence = assemblyline.ApplicationRequirementCandidateContentPresent
		nonRuntimePresence = assemblyline.ApplicationRequirementCandidateContentPresent
	default:
		t.Fatalf("unregistered candidate kind %q", kind)
	}
	result, resolved, err := assemblyline.ResolveApplicationRequirementCandidateKind(
		candidate,
		presence(assemblyline.ApplicationRequirementCandidateRuntimeContentDimension, runtimePresence),
		presence(assemblyline.ApplicationRequirementCandidateNonRuntimeContentDimension, nonRuntimePresence),
	)
	if err != nil || !resolved {
		t.Fatalf("resolve candidate kind %q: resolved=%t error=%v", kind, resolved, err)
	}
	return result
}
