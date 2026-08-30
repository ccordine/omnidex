package worker

import "github.com/gryph/omnidex/internal/assemblyline"

func replayRepositorySemanticLeaf(
	job assemblyline.PortableJob,
	raw string,
) (bool, error) {
	switch job.Kind {
	case assemblyline.WorkRepositoryRequirementInventory:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeRepositoryRequirementInventory)
	case assemblyline.WorkRepositoryRequirementCandidateAuthorization:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeRepositoryRequirementCandidateAuthorizationResult)
	case assemblyline.WorkRepositoryRequirementCandidateRelation:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeRepositoryRequirementCandidateRelationResult)
	case assemblyline.WorkRepositoryEvidenceRelevanceRelation:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeRepositoryEvidenceRelevanceRelationResult)
	case assemblyline.WorkRepositoryChangeOwner:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeRepositoryChangeOwnerLeaf)
	default:
		return false, nil
	}
}
