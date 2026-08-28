package worker

import "github.com/gryph/omnidex/internal/assemblyline"

func replayConversationSemanticLeaf(
	job assemblyline.PortableJob,
	raw string,
) (bool, error) {
	switch job.Kind {
	case assemblyline.WorkContextSearchTermCoverage:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeContextSearchTermCoverageLeaf)
	case assemblyline.WorkContextSearchTerm:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeContextSearchTermLeaf)
	case assemblyline.WorkContextRelevanceSelection:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeContextRelevanceSelectionDecision)
	case assemblyline.WorkContextMinification:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeContextMinificationDecision)
	case assemblyline.WorkConversationObjectiveKind:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeConversationObjectiveKindDecision)
	case assemblyline.WorkConversationResponse:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeConversationResponseDecision)
	case assemblyline.WorkRoleplayGroundedResponseText:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeRoleplayGroundedResponseTextLeaf)
	case assemblyline.WorkRoleplayGroundedResponseEvidenceRelation:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeRoleplayGroundedResponseEvidenceRelationLeaf)
	case assemblyline.WorkRoleplayCanonFactCoverage:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeRoleplayCanonFactCoverageLeaf)
	case assemblyline.WorkRoleplayCanonFact:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeRoleplayCanonFactLeaf)
	case assemblyline.WorkRoleplayOngoingAction:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeRoleplayOngoingActionDecision)
	case assemblyline.WorkGroundedAnswerText:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeGroundedAnswerTextDecision)
	case assemblyline.WorkGroundedAnswerEvidenceRelation:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeGroundedAnswerEvidenceRelationDecision)
	default:
		return false, nil
	}
}
