package worker

import "github.com/gryph/omnidex/internal/assemblyline"

func partitionDirectCodingApplicationRequirementCandidate(
	runtime typedWorkerRuntime,
	intentModel string,
	input assemblyline.ApplicationRequirementCandidatePartitionInput,
	identities []assemblyline.ArtifactIdentity,
) (assemblyline.ApplicationRequirementCandidatePartition, error) {
	job, err := assemblyline.NewApplicationRequirementCandidatePartitionJob(input)
	if err != nil {
		return assemblyline.ApplicationRequirementCandidatePartition{}, err
	}
	return runDirectCodingSemanticLeafCall(
		runtime,
		intentModel,
		"application_requirement_candidate_partition",
		job,
		identities,
		func(raw string) (assemblyline.ApplicationRequirementCandidatePartition, error) {
			return assemblyline.DecodeApplicationRequirementCandidatePartition(input, raw)
		},
		func(value assemblyline.ApplicationRequirementCandidatePartition) error {
			return value.ValidateFor(input)
		},
	)
}
