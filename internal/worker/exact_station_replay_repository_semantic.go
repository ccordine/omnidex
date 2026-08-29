package worker

import "github.com/gryph/omnidex/internal/assemblyline"

func replayRepositorySemanticLeaf(
	job assemblyline.PortableJob,
	raw string,
) (bool, error) {
	switch job.Kind {
	case assemblyline.WorkRepositoryRequirementCoverage:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeRepositoryRequirementCoverageLeaf)
	case assemblyline.WorkRepositoryRequirement:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeRepositoryRequirementLeaf)
	case assemblyline.WorkRepositoryEvidenceRelevanceLeaf:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeRepositoryEvidenceRelevanceLeaf)
	case assemblyline.WorkRepositoryChangeOwner:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeRepositoryChangeOwnerLeaf)
	default:
		return false, nil
	}
}
