package objectiveworkload

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func buildWorkload(
	authority Authority,
	decision assemblyline.RequirementPartitionDecision,
) (Workload, error) {
	if err := assemblyline.ValidateCompleteRequirementPartition(authority.Text, decision); err != nil {
		return Workload{}, fmt.Errorf("%w: %v", ErrInvalidGraph, err)
	}
	if len(decision.FeatureQuotes) > maxRequirements {
		return Workload{}, fmt.Errorf("%w: exceeds %d requirements", ErrInvalidGraph, maxRequirements)
	}
	requirements := make([]Requirement, len(decision.FeatureQuotes))
	for index, quote := range decision.FeatureQuotes {
		start := strings.Index(authority.Text, quote)
		if start < 0 || strings.Contains(authority.Text[start+1:], quote) {
			return Workload{}, fmt.Errorf("%w: quote %q is not uniquely grounded", ErrInvalidGraph, quote)
		}
		requirements[index] = Requirement{
			ID: RequirementID(fmt.Sprintf("R%03d", index+1)), SourceQuote: quote,
			Start: start, End: start + len(quote), SHA256: digestBytes([]byte(quote)),
		}
	}
	workload := Workload{
		ID: compiledWorkloadIdentity(authority, requirements), Authority: authority,
		RootObjectiveID: "O000_root", Requirements: requirements,
		Objectives: expectedObjectives(requirements, ObjectivePending),
	}
	if err := validateWorkload(workload, true); err != nil {
		return Workload{}, err
	}
	return workload, nil
}
