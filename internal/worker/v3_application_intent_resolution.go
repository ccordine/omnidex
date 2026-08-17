package worker

import "github.com/gryph/omnidex/internal/assemblyline"

func resolveDirectCodingApplicationIntent(
	runtime typedWorkerRuntime,
	intentModel string,
	authority assemblyline.ApplicationIntentInput,
	identities []assemblyline.ArtifactIdentity,
) (assemblyline.ApplicationIntentResolution, error) {
	var zero assemblyline.ApplicationIntentResolution
	job, err := assemblyline.NewApplicationIntentJob(authority)
	if err != nil {
		return zero, err
	}
	retained, err := runDirectCodingSemanticCall[assemblyline.ApplicationIntentCandidate](
		runtime, intentModel, "application_intent", job, identities,
		func(value assemblyline.ApplicationIntentCandidate) error { return value.Validate() },
	)
	if err != nil {
		return zero, err
	}
	return assemblyline.ResolveApplicationIntent(authority, retained)
}
