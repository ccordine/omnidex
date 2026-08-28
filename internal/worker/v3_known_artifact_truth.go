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
	return runDirectCodingSemanticLeafCall(
		runtime, modelName, "known_artifact_truth", job, identities,
		func(raw string) (assemblyline.KnownArtifactTruthDecision, error) {
			return assemblyline.DecodeKnownArtifactTruthDecision(input, raw)
		},
		func(value assemblyline.KnownArtifactTruthDecision) error {
			if err := value.ValidateFor(input); err != nil {
				return fmt.Errorf("validate known artifact truth: %w", err)
			}
			return nil
		},
	)
}

type knownArtifactTruthPartition struct {
	MustBeAbsent  []string
	MustExist     []string
	NotApplicable []string
}

func classifyKnownArtifactTruthQuotes(
	runtime typedWorkerRuntime,
	modelName string,
	quotes []string,
	identities []assemblyline.ArtifactIdentity,
) (knownArtifactTruthPartition, error) {
	partition := knownArtifactTruthPartition{
		MustBeAbsent:  make([]string, 0, len(quotes)),
		MustExist:     make([]string, 0, len(quotes)),
		NotApplicable: make([]string, 0, len(quotes)),
	}
	for _, quote := range quotes {
		decision, err := classifyKnownArtifactTruth(
			runtime, modelName,
			assemblyline.KnownArtifactTruthInput{RequirementQuote: quote}, identities,
		)
		if err != nil {
			return knownArtifactTruthPartition{}, err
		}
		switch decision.Truth {
		case assemblyline.KnownArtifactMustBeAbsent:
			partition.MustBeAbsent = append(partition.MustBeAbsent, quote)
		case assemblyline.OnePlainTextArtifactMustExist:
			partition.MustExist = append(partition.MustExist, quote)
		case assemblyline.KnownArtifactTruthNotApplicable:
			partition.NotApplicable = append(partition.NotApplicable, quote)
		default:
			return knownArtifactTruthPartition{}, fmt.Errorf(
				"known artifact truth %q escaped validated authority", decision.Truth,
			)
		}
	}
	return partition, nil
}
