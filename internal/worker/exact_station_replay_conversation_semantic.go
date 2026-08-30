package worker

import "github.com/gryph/omnidex/internal/assemblyline"

func replayConversationSemanticLeaf(
	job assemblyline.PortableJob,
	raw string,
) (bool, error) {
	switch job.Kind {
	case assemblyline.WorkContextRelevanceRelation:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeContextRelevanceRelationResult)
	case assemblyline.WorkContextMinification:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeContextMinificationDecision)
	case assemblyline.WorkConversationObjectiveKind:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeConversationObjectiveKindDecision)
	case assemblyline.WorkConversationResponse:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeConversationResponseDecision)
	case assemblyline.WorkRoleplayGroundedResponseParagraphInventory:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeRoleplayGroundedParagraphInventory)
	case assemblyline.WorkRoleplayGroundedResponseEvidenceRelation:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeRoleplayGroundedResponseEvidenceRelationLeaf)
	case assemblyline.WorkRoleplayGroundedResponseParagraphAuthorization:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeRoleplayGroundedParagraphAuthorizationDecision)
	case assemblyline.WorkRoleplayCanonFactInventory:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeRoleplayCanonFactInventory)
	case assemblyline.WorkRoleplayCanonFactCandidateAuthorization:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeRoleplayCanonFactCandidateAuthorization)
	case assemblyline.WorkRoleplayCanonFactCandidateRelation:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeRoleplayCanonFactCandidateRelation)
	case assemblyline.WorkRoleplayOngoingAction:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeRoleplayOngoingActionDecision)
	case assemblyline.WorkGroundedAnswerParagraphInventory:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeGroundedAnswerParagraphInventory)
	case assemblyline.WorkGroundedAnswerParagraphEvidenceRelation:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeGroundedAnswerParagraphEvidenceRelationDecision)
	case assemblyline.WorkGroundedAnswerParagraphAuthorization:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeGroundedAnswerParagraphAuthorizationDecision)
	default:
		return false, nil
	}
}
