package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const maxDirectCodingRequirementRefinements = 128

func extractRequirementFeatureEnvelopes(
	runtime typedWorkerRuntime,
	modelName string,
	authority string,
	identities []assemblyline.ArtifactIdentity,
) ([]string, int, error) {
	input := assemblyline.RequirementPartitionInput{
		SourceText: authority, Mode: assemblyline.RequirementExtractFeatures,
	}
	decision, err := partitionRequirementFeatures(runtime, modelName, input, 1, identities)
	if err != nil {
		return nil, 0, err
	}
	return decision.FeatureQuotes, 1, nil
}

func refineRequirementFeature(
	runtime typedWorkerRuntime,
	modelName string,
	featureEnvelope string,
	sequence *int,
	identities []assemblyline.ArtifactIdentity,
) ([]string, error) {
	queue := []string{featureEnvelope}
	features := make([]string, 0, 1)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if err := advanceRequirementPartitionSequence(sequence); err != nil {
			return nil, err
		}
		input := assemblyline.RequirementPartitionInput{
			SourceText: current, Mode: assemblyline.RequirementSplitFeature,
		}
		partition, err := partitionRequirementFeatures(runtime, modelName, input, *sequence, identities)
		if err != nil {
			return nil, err
		}
		if len(partition.FeatureQuotes) == 1 && partition.FeatureQuotes[0] == current {
			features = append(features, current)
			continue
		}
		for _, child := range partition.FeatureQuotes {
			if len(child) >= len(current) {
				return nil, fmt.Errorf(
					"requirement feature split %q did not make strict progress from %q", child, current,
				)
			}
			queue = append(queue, child)
		}
	}
	return features, nil
}

func advanceRequirementPartitionSequence(sequence *int) error {
	if sequence == nil {
		return fmt.Errorf("requirement partition sequence is required")
	}
	if *sequence >= maxDirectCodingRequirementRefinements {
		return fmt.Errorf(
			"requirement refinement exceeded %d bounded partition jobs",
			maxDirectCodingRequirementRefinements,
		)
	}
	*sequence++
	return nil
}

func partitionRequirementFeatures(
	runtime typedWorkerRuntime,
	modelName string,
	input assemblyline.RequirementPartitionInput,
	iteration int,
	identities []assemblyline.ArtifactIdentity,
) (assemblyline.RequirementPartitionDecision, error) {
	job, err := assemblyline.NewRequirementPartitionJob(input)
	if err != nil {
		return assemblyline.RequirementPartitionDecision{}, err
	}
	subject := fmt.Sprintf("requirement_extraction_%03d", iteration)
	if input.Mode == assemblyline.RequirementSplitFeature {
		subject = fmt.Sprintf("requirement_split_%03d", iteration)
	}
	return runDirectCodingSemanticCall[assemblyline.RequirementPartitionDecision](
		runtime, modelName, subject, job, identities,
		func(value assemblyline.RequirementPartitionDecision) error { return value.ValidateFor(input) },
	)
}
