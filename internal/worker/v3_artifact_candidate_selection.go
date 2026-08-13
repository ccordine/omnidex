package worker

import "github.com/gryph/omnidex/internal/assemblyline"

// selectArtifactCandidate crosses one path-blind semantic gap. Candidate
// acquisition, repository identity, and physical validation remain in the
// separate code-owned authority compiler.
func selectArtifactCandidate(
	runtime typedWorkerRuntime,
	modelName string,
	input assemblyline.ArtifactCandidateSelectionInput,
	identities []assemblyline.ArtifactIdentity,
) (assemblyline.ArtifactCandidateSelectionDecision, error) {
	job, err := assemblyline.NewArtifactCandidateSelectionJob(input)
	if err != nil {
		return assemblyline.ArtifactCandidateSelectionDecision{}, err
	}
	return runDirectCodingSemanticCall[assemblyline.ArtifactCandidateSelectionDecision](
		runtime, modelName, "artifact_candidate_selection", job, identities,
		func(value assemblyline.ArtifactCandidateSelectionDecision) error {
			return value.ValidateFor(input)
		},
	)
}
