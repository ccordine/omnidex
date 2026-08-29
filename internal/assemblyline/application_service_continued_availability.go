package assemblyline

import "fmt"

const ApplicationServiceContinuedAvailabilitySchemaV1 = "omnidex.application-service-continued-availability.v1"

type ApplicationServiceContinuedAvailabilityCandidateID string

const (
	ApplicationServiceAvailabilityNotRequiredCandidate ApplicationServiceContinuedAvailabilityCandidateID = "AVAILABILITY_CANDIDATE_1"
	ApplicationServiceAvailabilityRequiredCandidate    ApplicationServiceContinuedAvailabilityCandidateID = "AVAILABILITY_CANDIDATE_2"
)

type ApplicationServiceContinuedAvailabilityInput struct {
	UserRequest string `json:"user_request"`
}

type ApplicationServiceContinuedAvailabilityResult struct {
	Schema      string                                             `json:"schema"`
	CandidateID ApplicationServiceContinuedAvailabilityCandidateID `json:"candidate_id"`
}

func NewApplicationServiceContinuedAvailabilityJob(
	input ApplicationServiceContinuedAvailabilityInput,
) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationServiceContinuedAvailability, input, input.validate)
}

func (input ApplicationServiceContinuedAvailabilityInput) validate() error {
	if err := validateApplicationRequest("application service continued availability", input.UserRequest); err != nil {
		return err
	}
	return ValidatePathFreeModelContext(
		"application service continued availability request", input.UserRequest,
	)
}

func (result ApplicationServiceContinuedAvailabilityResult) ValidateFor(
	input ApplicationServiceContinuedAvailabilityInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != ApplicationServiceContinuedAvailabilitySchemaV1 {
		return fmt.Errorf(
			"application service continued availability schema must be %q",
			ApplicationServiceContinuedAvailabilitySchemaV1,
		)
	}
	switch result.CandidateID {
	case ApplicationServiceAvailabilityNotRequiredCandidate,
		ApplicationServiceAvailabilityRequiredCandidate:
		return nil
	default:
		return fmt.Errorf(
			"application service continued availability candidate %q is unavailable",
			result.CandidateID,
		)
	}
}
