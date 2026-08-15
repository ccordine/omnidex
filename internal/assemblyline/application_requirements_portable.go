package assemblyline

import "fmt"

// DecodeApplicationRequirementInterpretationResult restores the frozen user
// authority carried by a portable requirements job before resolving its
// untrusted final response through the production grounding boundary.
func DecodeApplicationRequirementInterpretationResult(
	job PortableJob,
	raw string,
) (ApplicationRequirementResolution, error) {
	var zero ApplicationRequirementResolution
	if err := job.Validate(); err != nil {
		return zero, err
	}
	if job.Kind != WorkApplicationRequirements {
		return zero, fmt.Errorf(
			"application requirement interpretation result requires work kind %q",
			WorkApplicationRequirements,
		)
	}
	var input ApplicationRequirementInterpretationInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return zero, err
	}
	var interpretation ApplicationRequirementInterpretation
	if err := decodePortablePayload([]byte(raw), &interpretation); err != nil {
		return zero, fmt.Errorf("decode application requirement interpretation: %w", err)
	}
	return ResolveApplicationRequirements(input, interpretation)
}
