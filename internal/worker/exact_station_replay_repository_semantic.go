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
	case assemblyline.WorkRepositorySearchAnchorCoverage:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeRepositorySearchAnchorCoverageLeaf)
	case assemblyline.WorkRepositorySearchAnchor:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeRepositorySearchAnchorLeaf)
	case assemblyline.WorkRepositoryEvidenceRelevanceLeaf:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeRepositoryEvidenceRelevanceLeaf)
	case assemblyline.WorkRepositoryChangeOwner:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeRepositoryChangeOwnerLeaf)
	case assemblyline.WorkRepositoryGroundedIssueDetail:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeRepositoryGroundedIssueDetailLeaf)
	case assemblyline.WorkRepositoryGroundedIssueKind:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeRepositoryGroundedIssueKindLeaf)
	case assemblyline.WorkRepositoryGroundedCorrection:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeRepositoryGroundedCorrectionDecision)
	default:
		return false, nil
	}
}
