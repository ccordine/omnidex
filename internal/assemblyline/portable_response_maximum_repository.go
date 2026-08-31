package assemblyline

import "github.com/gryph/omnidex/internal/roleplay"

func portableRepositoryConversationResponseMaximum(job PortableJob) (int, bool, error) {
	switch job.Kind {
	case WorkRepositoryRequirementInventory:
		return maxRepositoryRequirementInventoryBytes, true, nil
	case WorkRepositoryRequirementCandidateAuthorization:
		return maximumStringBytes(
			RepositoryRequirementCandidateRequiresChange,
			RepositoryRequirementCandidateNoChange,
		), true, nil
	case WorkRepositoryRequirementCandidateRelation:
		return maximumStringBytes(
			RepositoryRequirementCandidatesSameChange,
			RepositoryRequirementCandidatesDistinctChanges,
		), true, nil
	case WorkContextRelevanceRelation:
		return maximumStringBytes(
			ContextCandidateDirectlyRelevant,
			ContextCandidateNotDirectlyRelevant,
		), true, nil
	case WorkContextMinification:
		return MaxContextMinifiedBytes, true, nil
	case WorkConversationObjectiveKind:
		return maximumStringBytes(
			ObjectiveKindAnswer, ObjectiveKindWorkspaceMutation, ObjectiveKindExternalAnswer,
			ObjectiveKindStory, ObjectiveKindDatabaseRead,
		), true, nil
	case WorkConversationResponse:
		var input ConversationResponseInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return 0, true, err
		}
		if input.RoleplayIdentity != nil {
			return roleplay.MaxNarrativeResponseBytes, true, nil
		}
		return maxConversationResponseTextBytes, true, nil
	case WorkRoleplayGroundedResponseParagraphInventory:
		return max(
			maxRoleplayGroundedParagraphInventoryBytes,
			len(RoleplayNoGroundedParagraphCandidates),
		), true, nil
	case WorkRoleplayGroundedResponseEvidenceRelation:
		return maximumStringBytes(
			RoleplayGroundedEvidenceSupportsParagraph,
			RoleplayGroundedEvidenceDoesNotSupport,
		), true, nil
	case WorkRoleplayGroundedResponseParagraphAuthorization:
		return maximumStringBytes(
			RoleplayGroundedParagraphResponsiveAndSupported,
			RoleplayGroundedParagraphNotAuthorized,
		), true, nil
	case WorkRoleplayCanonFactInventory:
		return maxRoleplayCanonFactInventoryBytes, true, nil
	case WorkRoleplayCanonFactCandidateAuthorization:
		return maximumStringBytes(
			RoleplayCanonFactEstablished, RoleplayCanonFactNotEstablished,
		), true, nil
	case WorkRoleplayCanonFactCandidateRelation:
		return maximumStringBytes(
			RoleplayCanonFactsEquivalent, RoleplayCanonFactsDistinct,
		), true, nil
	case WorkRoleplayOngoingAction:
		return roleplay.MaxOngoingActionBytes, true, nil
	case WorkGroundedAnswerParagraphInventory:
		return max(
			maxGroundedAnswerParagraphInventoryBytes,
			len(GroundedAnswerNoParagraphCandidates),
		), true, nil
	case WorkGroundedAnswerParagraphEvidenceRelation:
		return maximumStringBytes(
			GroundedEvidenceSupportsParagraph, GroundedEvidenceDoesNotSupport,
		), true, nil
	case WorkGroundedAnswerParagraphAuthorization:
		return maximumStringBytes(
			GroundedParagraphResponsiveAndFullySupported,
			GroundedParagraphNotResponsiveOrUnsupported,
		), true, nil
	default:
		return 0, false, nil
	}
}
