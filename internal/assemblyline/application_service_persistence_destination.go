package assemblyline

import "fmt"

const ApplicationServicePersistenceDestinationSchemaV1 = "omnidex.application-service-persistence-destination.v1"

type ApplicationServicePersistenceDestinationCandidateID string

const (
	ApplicationServiceBuildEnvironmentDestinationCandidate    ApplicationServicePersistenceDestinationCandidateID = "DESTINATION_CANDIDATE_1"
	ApplicationServiceBuildEnvironmentNotEstablishedCandidate ApplicationServicePersistenceDestinationCandidateID = "DESTINATION_CANDIDATE_2"
)

type ApplicationServicePersistenceDestinationInput struct {
	UserRequest           string                                        `json:"user_request"`
	ContinuedAvailability ApplicationServiceContinuedAvailabilityResult `json:"continued_availability"`
}

type ApplicationServicePersistenceDestinationResult struct {
	Schema      string                                              `json:"schema"`
	CandidateID ApplicationServicePersistenceDestinationCandidateID `json:"candidate_id"`
}

func NewApplicationServicePersistenceDestinationJob(
	input ApplicationServicePersistenceDestinationInput,
) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationServicePersistenceDestination, input, input.validate)
}

func (input ApplicationServicePersistenceDestinationInput) validate() error {
	if err := validateApplicationRequest("application service persistence destination", input.UserRequest); err != nil {
		return err
	}
	if err := ValidatePathFreeModelContext(
		"application service persistence destination request", input.UserRequest,
	); err != nil {
		return err
	}
	availabilityInput := ApplicationServiceContinuedAvailabilityInput{
		UserRequest: input.UserRequest,
	}
	if err := input.ContinuedAvailability.ValidateFor(availabilityInput); err != nil {
		return fmt.Errorf("validate persistence destination availability authority: %w", err)
	}
	if input.ContinuedAvailability.CandidateID != ApplicationServiceAvailabilityRequiredCandidate {
		return fmt.Errorf(
			"persistence destination requires explicit continued-availability authority",
		)
	}
	return nil
}

func (result ApplicationServicePersistenceDestinationResult) ValidateFor(
	input ApplicationServicePersistenceDestinationInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != ApplicationServicePersistenceDestinationSchemaV1 {
		return fmt.Errorf(
			"application service persistence destination schema must be %q",
			ApplicationServicePersistenceDestinationSchemaV1,
		)
	}
	switch result.CandidateID {
	case ApplicationServiceBuildEnvironmentDestinationCandidate,
		ApplicationServiceBuildEnvironmentNotEstablishedCandidate:
		return nil
	default:
		return fmt.Errorf(
			"application service persistence destination candidate %q is unavailable",
			result.CandidateID,
		)
	}
}
