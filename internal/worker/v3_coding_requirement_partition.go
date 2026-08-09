package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

// partitionCodingRequirements is the one production requirement-partition
// boundary for both greenfield and existing-repository coding. Code owns the
// extraction and split fixed point; the configured stable semantic model owns
// only one schema-bound partition decision per call.
func partitionCodingRequirements(
	runtime typedWorkerRuntime,
	modelName string,
	authority string,
	identities []assemblyline.ArtifactIdentity,
) (assemblyline.RequirementPartitionDecision, error) {
	sequence := 0
	return assemblyline.CompleteRequirementPartition(
		authority,
		func(input assemblyline.RequirementPartitionInput) (assemblyline.RequirementPartitionDecision, error) {
			sequence++
			job, err := assemblyline.NewRequirementPartitionJob(input)
			if err != nil {
				return assemblyline.RequirementPartitionDecision{}, err
			}
			subject := fmt.Sprintf("requirement_extract_%03d", sequence)
			if input.Mode == assemblyline.RequirementSplitFeature {
				subject = fmt.Sprintf("requirement_split_%03d", sequence)
			}
			return runDirectCodingSemanticCall[assemblyline.RequirementPartitionDecision](
				runtime, modelName, subject, job, identities,
				func(value assemblyline.RequirementPartitionDecision) error {
					return value.ValidateFor(input)
				},
			)
		},
	)
}
