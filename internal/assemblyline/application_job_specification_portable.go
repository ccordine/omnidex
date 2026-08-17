package assemblyline

import "fmt"

// DecodeApplicationJobSpecificationResult restores the frozen authority from
// one portable planning job and validates its untrusted final response.
func DecodeApplicationJobSpecificationResult(
	job PortableJob,
	raw string,
) (ApplicationJobSpecification, error) {
	var zero ApplicationJobSpecification
	if err := job.Validate(); err != nil {
		return zero, err
	}
	if job.Kind != WorkApplicationJobSpecification {
		return zero, fmt.Errorf(
			"application job specification result requires work kind %q",
			WorkApplicationJobSpecification,
		)
	}
	var input ApplicationJobSpecificationInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return zero, err
	}
	specification, err := DecodeApplicationJobSpecification(input, raw)
	if err != nil {
		return zero, err
	}
	if err := ValidateApplicationJobSpecification(specification); err != nil {
		return zero, err
	}
	return specification, nil
}
