package assemblyline

import "fmt"

const ApplicationServiceDeploymentIntentSchemaV1 = "omnidex.application-service-deployment-intent.v1"

type ApplicationServiceDeploymentCandidateID string

const (
	ApplicationServiceDeploymentNoPersistenceCandidate ApplicationServiceDeploymentCandidateID = "DEPLOYMENT_CANDIDATE_1"
	ApplicationServiceDeploymentCurrentHostCandidate   ApplicationServiceDeploymentCandidateID = "DEPLOYMENT_CANDIDATE_2"
	ApplicationServiceDeploymentOtherTargetCandidate   ApplicationServiceDeploymentCandidateID = "DEPLOYMENT_CANDIDATE_3"
)

type ApplicationServiceDeploymentIntentInput struct {
	UserRequest string `json:"user_request"`
}

type ApplicationServiceDeploymentIntentResult struct {
	Schema      string                                  `json:"schema"`
	CandidateID ApplicationServiceDeploymentCandidateID `json:"candidate_id"`
}

type ApplicationServiceDeploymentDisposition string

const (
	ApplicationServiceDeploymentVerifyOnly         ApplicationServiceDeploymentDisposition = "verify_only"
	ApplicationServiceDeploymentPersistCurrentHost ApplicationServiceDeploymentDisposition = "persist_current_host"
	ApplicationServiceDeploymentTargetUnresolved   ApplicationServiceDeploymentDisposition = "target_unresolved"
)

func NewApplicationServiceDeploymentIntentJob(
	input ApplicationServiceDeploymentIntentInput,
) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationServiceDeploymentIntent, input, input.validate)
}

func (input ApplicationServiceDeploymentIntentInput) validate() error {
	if err := validateApplicationRequest("application service deployment intent", input.UserRequest); err != nil {
		return err
	}
	return ValidatePathFreeModelContext(
		"application service deployment intent request", input.UserRequest,
	)
}

func (result ApplicationServiceDeploymentIntentResult) ValidateFor(
	input ApplicationServiceDeploymentIntentInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != ApplicationServiceDeploymentIntentSchemaV1 {
		return fmt.Errorf(
			"application service deployment intent schema must be %q",
			ApplicationServiceDeploymentIntentSchemaV1,
		)
	}
	switch result.CandidateID {
	case ApplicationServiceDeploymentNoPersistenceCandidate,
		ApplicationServiceDeploymentCurrentHostCandidate,
		ApplicationServiceDeploymentOtherTargetCandidate:
		return nil
	default:
		return fmt.Errorf(
			"application service deployment candidate %q is unavailable",
			result.CandidateID,
		)
	}
}

func ResolveApplicationServiceDeploymentDisposition(
	result ApplicationServiceDeploymentIntentResult,
) (ApplicationServiceDeploymentDisposition, error) {
	if result.Schema != ApplicationServiceDeploymentIntentSchemaV1 {
		return "", fmt.Errorf(
			"application service deployment intent schema must be %q",
			ApplicationServiceDeploymentIntentSchemaV1,
		)
	}
	switch result.CandidateID {
	case ApplicationServiceDeploymentNoPersistenceCandidate:
		return ApplicationServiceDeploymentVerifyOnly, nil
	case ApplicationServiceDeploymentCurrentHostCandidate:
		return ApplicationServiceDeploymentPersistCurrentHost, nil
	case ApplicationServiceDeploymentOtherTargetCandidate:
		return ApplicationServiceDeploymentTargetUnresolved, nil
	default:
		return "", fmt.Errorf(
			"application service deployment candidate %q is unavailable",
			result.CandidateID,
		)
	}
}
