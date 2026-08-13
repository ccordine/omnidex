package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func classifyKnownArtifactTruth(
	runtime typedWorkerRuntime,
	modelName string,
	input assemblyline.KnownArtifactTruthInput,
	identities []assemblyline.ArtifactIdentity,
) (assemblyline.KnownArtifactTruthDecision, error) {
	job, err := assemblyline.NewKnownArtifactTruthJob(input)
	if err != nil {
		return assemblyline.KnownArtifactTruthDecision{}, err
	}
	return runDirectCodingSemanticCall[assemblyline.KnownArtifactTruthDecision](
		runtime, modelName, "known_artifact_truth", job, identities,
		func(value assemblyline.KnownArtifactTruthDecision) error {
			if err := value.ValidateFor(input); err != nil {
				return fmt.Errorf("validate known artifact truth: %w", err)
			}
			return nil
		},
	)
}

func classifyKnownArtifactTruthQuotes(
	runtime typedWorkerRuntime,
	modelName string,
	quotes []string,
	identities []assemblyline.ArtifactIdentity,
) ([]string, error) {
	absent := make([]string, 0, len(quotes))
	for _, quote := range quotes {
		decision, err := classifyKnownArtifactTruth(
			runtime, modelName,
			assemblyline.KnownArtifactTruthInput{RequirementQuote: quote}, identities,
		)
		if err != nil {
			return nil, err
		}
		if decision.Truth == assemblyline.KnownArtifactMustBeAbsent {
			absent = append(absent, quote)
		}
	}
	return absent, nil
}
