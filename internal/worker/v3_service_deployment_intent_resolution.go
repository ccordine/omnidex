package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/exactjson"
)

type directCodingServiceDeploymentResolution struct {
	Disposition    assemblyline.ApplicationServiceDeploymentDisposition
	IntentJobID    string
	ResponseSHA256 string
}

func resolveDirectCodingServiceDeploymentDisposition(
	runtime typedWorkerRuntime,
	model string,
	userRequest string,
	identities []assemblyline.ArtifactIdentity,
) (directCodingServiceDeploymentResolution, error) {
	input := assemblyline.ApplicationServiceDeploymentIntentInput{UserRequest: userRequest}
	job, err := assemblyline.NewApplicationServiceDeploymentIntentJob(input)
	if err != nil {
		return directCodingServiceDeploymentResolution{}, err
	}
	runtime.MaxAttempts = 1
	runtime.CorrectionModel = ""
	result, err := runDirectCodingSemanticCall[assemblyline.ApplicationServiceDeploymentIntentResult](
		runtime, model, "application_service_deployment_intent", job, identities,
		func(value assemblyline.ApplicationServiceDeploymentIntentResult) error {
			return value.ValidateFor(input)
		},
	)
	if err != nil {
		return directCodingServiceDeploymentResolution{}, fmt.Errorf("resolve service deployment disposition: %w", err)
	}
	disposition, err := assemblyline.ResolveApplicationServiceDeploymentDisposition(result)
	if err != nil {
		return directCodingServiceDeploymentResolution{}, err
	}
	if disposition == assemblyline.ApplicationServiceDeploymentTargetUnresolved {
		return directCodingServiceDeploymentResolution{}, fmt.Errorf(
			"requested persistent deployment target is outside the registered current-host authority",
		)
	}
	canonical, err := exactjson.Canonical(result)
	if err != nil {
		return directCodingServiceDeploymentResolution{}, fmt.Errorf("encode accepted service deployment intent: %w", err)
	}
	digest := sha256.Sum256(canonical)
	return directCodingServiceDeploymentResolution{
		Disposition: disposition, IntentJobID: job.ID,
		ResponseSHA256: hex.EncodeToString(digest[:]),
	}, nil
}
