package worker

import "github.com/gryph/omnidex/internal/assemblyline"

func replayGeneralSemanticLeaf(
	job assemblyline.PortableJob,
	raw string,
) (bool, error) {
	switch job.Kind {
	case assemblyline.WorkArtifactHandling:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeArtifactHandlingDecision)
	case assemblyline.WorkRepositoryArtifactAbsence:
		return true, decodeReplaySemanticLeaf(
			job, raw, assemblyline.DecodeRepositoryArtifactAbsenceDecision,
		)
	case assemblyline.WorkPlainTextArtifactCreation:
		return true, decodeReplaySemanticLeaf(
			job, raw, assemblyline.DecodePlainTextArtifactCreationDecision,
		)
	case assemblyline.WorkDeclarationArtifactBoundary:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeDeclarationArtifactBoundaryDecision)
	case assemblyline.WorkArtifactCandidateSelection:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeArtifactCandidateSelectionDecision)
	case assemblyline.WorkCapabilityRelation:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeCapabilityRelationDecision)
	case assemblyline.WorkSkillSelection:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeSkillSelectionDecision)
	default:
		return false, nil
	}
}
