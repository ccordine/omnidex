package assemblyline

import (
	"fmt"
	"strings"
)

func applicationRequirementCoverageProjection(
	input ApplicationRequirementCoverageInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	var projection strings.Builder
	projection.WriteString(renderApplicationContextModelProjection(input.UserRequest, input.Context))
	projection.WriteByte('\n')
	if len(input.AcceptedRequirements) == 0 {
		projection.WriteString("ACCEPTED REQUIREMENTS:\n(none)\n")
	} else {
		for index, requirement := range input.AcceptedRequirements {
			fmt.Fprintf(&projection, "ACCEPTED REQUIREMENT %d:\n%s\n", index+1, requirement)
		}
	}
	if len(input.ZeroDeltas) == 0 {
		projection.WriteString("CODE-ESTABLISHED ZERO-DELTAS:\n(none)\n")
	} else {
		for index, evidence := range input.ZeroDeltas {
			fmt.Fprintf(
				&projection,
				"CODE-ESTABLISHED ZERO-DELTA %d:\n%s\nRETAINED IDENTITY: %s at zero-based index %d\n",
				index+1,
				evidence.Candidate,
				evidence.RetainedSet,
				evidence.RetainedIndex,
			)
		}
	}
	if projection.Len() > maxPortablePayloadBytes {
		return "", fmt.Errorf(
			"application requirement projection exceeds %d bytes",
			maxPortablePayloadBytes,
		)
	}
	return strings.TrimSuffix(projection.String(), "\n"), nil
}

func applicationRequirementGenerationProjection(
	input ApplicationRequirementCoverageInput,
) (string, error) {
	projection, err := applicationRequirementCoverageProjection(input)
	if err != nil {
		return "", err
	}
	var generation strings.Builder
	generation.WriteString(projection)
	generation.WriteByte('\n')
	if len(input.ExcludedCandidates) == 0 {
		generation.WriteString("EXCLUDED NON-RUNTIME CANDIDATES:\n(none)\n")
	} else {
		for index, candidate := range input.ExcludedCandidates {
			fmt.Fprintf(
				&generation,
				"EXCLUDED NON-RUNTIME CANDIDATE %d:\n%s\n",
				index+1,
				candidate,
			)
		}
	}
	if generation.Len() > maxPortablePayloadBytes {
		return "", fmt.Errorf(
			"application requirement generation projection exceeds %d bytes",
			maxPortablePayloadBytes,
		)
	}
	return strings.TrimSuffix(generation.String(), "\n"), nil
}
