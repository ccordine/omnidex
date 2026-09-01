package assemblyline

import "github.com/gryph/omnidex/internal/roleplay"

func portableRepositoryConversationResponseMaximum(job PortableJob) (int, bool, error) {
	switch job.Kind {
	case WorkContextRelevanceRelation:
		maximum, err := opaqueModelChoiceBuilderResponseMaximum(
			contextRelevanceRelationChoices,
		)
		return maximum, true, err
	case WorkContextMinification:
		return MaxContextMinifiedBytes, true, nil
	case WorkConversationObjectiveKind:
		var input ConversationObjectiveKindInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return 0, true, err
		}
		maximum, err := opaqueModelChoiceBuilderResponseMaximum(func() ([]OpaqueModelChoice, error) {
			return conversationObjectiveKindChoices(input)
		})
		return maximum, true, err
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
		return maxRoleplayGroundedParagraphInventoryBytes, true, nil
	case WorkRoleplayGroundedResponseEvidenceRelation:
		maximum, err := opaqueModelChoiceBuilderResponseMaximum(
			roleplayGroundedEvidenceRelationChoices,
		)
		return maximum, true, err
	case WorkRoleplayGroundedResponseParagraphAuthorization:
		maximum, err := opaqueModelChoiceBuilderResponseMaximum(
			roleplayGroundedParagraphAuthorizationChoices,
		)
		return maximum, true, err
	case WorkRoleplayCanonFactPresence:
		maximum, err := opaqueModelChoiceBuilderResponseMaximum(
			roleplayCanonFactPresenceChoices,
		)
		return maximum, true, err
	case WorkRoleplayCanonFactInventory:
		return maxRoleplayCanonFactInventoryBytes, true, nil
	case WorkRoleplayCanonFactCandidateAuthorization:
		maximum, err := opaqueModelChoiceBuilderResponseMaximum(
			roleplayCanonFactAuthorizationChoices,
		)
		return maximum, true, err
	case WorkRoleplayCanonFactCandidateRelation:
		maximum, err := opaqueModelChoiceBuilderResponseMaximum(
			roleplayCanonFactRelationChoices,
		)
		return maximum, true, err
	case WorkRoleplayOngoingActionRelation:
		return 1, true, nil
	case WorkRoleplayOngoingActionValue:
		return roleplay.MaxOngoingActionBytes, true, nil
	case WorkGroundedAnswerParagraphInventory:
		return maxGroundedAnswerParagraphInventoryBytes, true, nil
	case WorkGroundedAnswerParagraphEvidenceRelation:
		maximum, err := opaqueModelChoiceBuilderResponseMaximum(
			groundedAnswerParagraphEvidenceRelationChoices,
		)
		return maximum, true, err
	case WorkGroundedAnswerParagraphAuthorization:
		maximum, err := opaqueModelChoiceBuilderResponseMaximum(
			groundedAnswerParagraphAuthorizationChoices,
		)
		return maximum, true, err
	default:
		return 0, false, nil
	}
}
