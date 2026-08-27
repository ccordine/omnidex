package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/exactjson"
)

type directCodingServiceDeploymentResolution struct {
	Disposition                          assemblyline.ApplicationServiceDeploymentDisposition
	ContinuedAvailabilityJobID           string
	ContinuedAvailabilityResponseSHA256  string
	PersistenceDestinationJobID          string
	PersistenceDestinationResponseSHA256 string
	DispositionJobID                     string
	DispositionResponseSHA256            string
}

func resolveDirectCodingServiceDeploymentDisposition(
	runtime typedWorkerRuntime,
	continuedAvailabilityModel string,
	persistenceDestinationModel string,
	userRequest string,
	identities []assemblyline.ArtifactIdentity,
) (directCodingServiceDeploymentResolution, error) {
	availabilityInput := assemblyline.ApplicationServiceContinuedAvailabilityInput{
		UserRequest: userRequest,
	}
	availabilityJob, err := assemblyline.NewApplicationServiceContinuedAvailabilityJob(
		availabilityInput,
	)
	if err != nil {
		return directCodingServiceDeploymentResolution{}, err
	}
	runtime.MaxAttempts = 1
	runtime.CorrectionModel = ""
	availability, err := runDirectCodingSemanticCall[assemblyline.ApplicationServiceContinuedAvailabilityResult](
		runtime, continuedAvailabilityModel, "application_service_continued_availability",
		availabilityJob, identities,
		func(value assemblyline.ApplicationServiceContinuedAvailabilityResult) error {
			return value.ValidateFor(availabilityInput)
		},
	)
	if err != nil {
		return directCodingServiceDeploymentResolution{}, fmt.Errorf(
			"resolve service continued availability: %w", err,
		)
	}
	availabilitySHA256, err := directCodingDeploymentSemanticResultSHA256(availability)
	if err != nil {
		return directCodingServiceDeploymentResolution{}, fmt.Errorf(
			"encode accepted service continued availability: %w", err,
		)
	}
	resolution := directCodingServiceDeploymentResolution{
		ContinuedAvailabilityJobID:          availabilityJob.ID,
		ContinuedAvailabilityResponseSHA256: availabilitySHA256,
	}
	if availability.CandidateID == assemblyline.ApplicationServiceAvailabilityNotRequiredCandidate {
		resolution.Disposition, err = assemblyline.ResolveApplicationServiceDeploymentDisposition(
			availability, nil,
		)
		if err != nil {
			return directCodingServiceDeploymentResolution{}, err
		}
		resolution.DispositionJobID = availabilityJob.ID
		resolution.DispositionResponseSHA256 = availabilitySHA256
		return resolution, nil
	}

	destinationInput := assemblyline.ApplicationServicePersistenceDestinationInput{
		UserRequest: userRequest, ContinuedAvailability: availability,
	}
	destinationJob, err := assemblyline.NewApplicationServicePersistenceDestinationJob(destinationInput)
	if err != nil {
		return directCodingServiceDeploymentResolution{}, err
	}
	destination, err := runDirectCodingSemanticCall[assemblyline.ApplicationServicePersistenceDestinationResult](
		runtime, persistenceDestinationModel, "application_service_persistence_destination",
		destinationJob, identities,
		func(value assemblyline.ApplicationServicePersistenceDestinationResult) error {
			return value.ValidateFor(destinationInput)
		},
	)
	if err != nil {
		return directCodingServiceDeploymentResolution{}, fmt.Errorf(
			"resolve service persistence destination: %w", err,
		)
	}
	destinationSHA256, err := directCodingDeploymentSemanticResultSHA256(destination)
	if err != nil {
		return directCodingServiceDeploymentResolution{}, fmt.Errorf(
			"encode accepted service persistence destination: %w", err,
		)
	}
	resolution.PersistenceDestinationJobID = destinationJob.ID
	resolution.PersistenceDestinationResponseSHA256 = destinationSHA256
	resolution.Disposition, err = assemblyline.ResolveApplicationServiceDeploymentDisposition(
		availability, &destination,
	)
	if err != nil {
		return directCodingServiceDeploymentResolution{}, err
	}
	if resolution.Disposition == assemblyline.ApplicationServiceDeploymentTargetUnresolved {
		return directCodingServiceDeploymentResolution{}, fmt.Errorf(
			"requested persistent deployment target is outside the registered current-host authority",
		)
	}
	resolution.DispositionJobID = destinationJob.ID
	resolution.DispositionResponseSHA256 = destinationSHA256
	return resolution, nil
}

func directCodingDeploymentSemanticResultSHA256(value any) (string, error) {
	canonical, err := exactjson.Canonical(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}
