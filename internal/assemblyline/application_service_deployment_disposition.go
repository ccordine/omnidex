package assemblyline

import "fmt"

type ApplicationServiceDeploymentDisposition string

const (
	ApplicationServiceDeploymentVerifyOnly         ApplicationServiceDeploymentDisposition = "verify_only"
	ApplicationServiceDeploymentPersistCurrentHost ApplicationServiceDeploymentDisposition = "persist_current_host"
	ApplicationServiceDeploymentTargetUnresolved   ApplicationServiceDeploymentDisposition = "target_unresolved"
)

// ResolveApplicationServiceDeploymentDisposition applies the code-owned
// transition after the two independent semantic leaves have been validated.
// A destination result is forbidden unless continued availability is required,
// and is mandatory when continued availability is required.
func ResolveApplicationServiceDeploymentDisposition(
	availability ApplicationServiceContinuedAvailabilityResult,
	destination *ApplicationServicePersistenceDestinationResult,
) (ApplicationServiceDeploymentDisposition, error) {
	if availability.Schema != ApplicationServiceContinuedAvailabilitySchemaV1 {
		return "", fmt.Errorf(
			"application service continued availability schema must be %q",
			ApplicationServiceContinuedAvailabilitySchemaV1,
		)
	}
	switch availability.CandidateID {
	case ApplicationServiceAvailabilityNotRequiredCandidate:
		if destination != nil {
			return "", fmt.Errorf(
				"persistence destination is forbidden when continued availability is not required",
			)
		}
		return ApplicationServiceDeploymentVerifyOnly, nil
	case ApplicationServiceAvailabilityRequiredCandidate:
		if destination == nil {
			return "", fmt.Errorf(
				"continued availability requires one persistence destination result",
			)
		}
	default:
		return "", fmt.Errorf(
			"application service continued availability candidate %q is unavailable",
			availability.CandidateID,
		)
	}
	if destination.Schema != ApplicationServicePersistenceDestinationSchemaV1 {
		return "", fmt.Errorf(
			"application service persistence destination schema must be %q",
			ApplicationServicePersistenceDestinationSchemaV1,
		)
	}
	switch destination.CandidateID {
	case ApplicationServiceBuildEnvironmentDestinationCandidate:
		return ApplicationServiceDeploymentPersistCurrentHost, nil
	case ApplicationServiceBuildEnvironmentNotEstablishedCandidate:
		return ApplicationServiceDeploymentTargetUnresolved, nil
	default:
		return "", fmt.Errorf(
			"application service persistence destination candidate %q is unavailable",
			destination.CandidateID,
		)
	}
}
