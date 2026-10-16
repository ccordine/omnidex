package assemblyline

import "github.com/gryph/omnidex/internal/roleplay"

func portableRepositoryConversationResponseMaximum(job PortableJob) (portableResponseContract, bool, error) {
	switch job.Kind {
	case WorkContextRelevanceRelation:
		maximum, err := opaqueModelChoiceBuilderResponseContract(
			contextRelevanceRelationChoices,
		)
		return maximum, true, err
	case WorkContextMinification:
		return portableResponseContract{openText: true}, true, nil
	case WorkConversationObjectiveKind:
		var input ConversationObjectiveKindInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return portableResponseContract{}, true, err
		}
		maximum, err := opaqueModelChoiceBuilderResponseContract(func() ([]OpaqueModelChoice, error) {
			return conversationObjectiveKindChoices(input)
		})
		return maximum, true, err
	case WorkConversationResponse:
		var input ConversationResponseInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return portableResponseContract{}, true, err
		}
		if input.RoleplayIdentity != nil {
			return portableResponseContract{maximum: roleplay.MaxNarrativeResponseBytes}, true, nil
		}
		return portableResponseContract{maximum: maxConversationResponseTextBytes}, true, nil
	case WorkRoleplayGroundedResponseParagraphInventory:
		return portableResponseContract{maximum: maxRoleplayGroundedParagraphInventoryBytes}, true, nil
	case WorkRoleplayGroundedResponseEvidenceRelation:
		maximum, err := opaqueModelChoiceBuilderResponseContract(
			roleplayGroundedEvidenceRelationChoices,
		)
		return maximum, true, err
	case WorkGroundedParagraphSupport:
		var input GroundedParagraphSupportInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return portableResponseContract{}, true, err
		}
		maximum, err := opaqueModelChoiceBuilderResponseContract(func() ([]OpaqueModelChoice, error) {
			return groundedParagraphSupportChoices(input.ClaimDomain)
		})
		return maximum, true, err
	case WorkRoleplayCanonFactInventory:
		return portableResponseContract{maximum: maxRoleplayCanonFactInventoryBytes}, true, nil
	case WorkRoleplayCanonFactCandidateAuthorization:
		maximum, err := opaqueModelChoiceBuilderResponseContract(
			roleplayCanonFactAuthorizationChoices,
		)
		return maximum, true, err
	case WorkRoleplayCanonFactCandidateRelation:
		maximum, err := opaqueModelChoiceBuilderResponseContract(
			roleplayCanonFactRelationChoices,
		)
		return maximum, true, err
	case WorkRoleplayOngoingActionRelation:
		var input RoleplayOngoingActionRelationInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return portableResponseContract{}, true, err
		}
		contract, err := opaqueModelChoiceBuilderResponseContract(func() ([]OpaqueModelChoice, error) {
			return roleplayOngoingActionRelationChoices(input)
		})
		return contract, true, err
	case WorkRoleplayOngoingActionValue:
		return portableResponseContract{maximum: roleplay.MaxOngoingActionBytes}, true, nil
	case WorkGroundedAnswerParagraphInventory:
		return portableResponseContract{maximum: maxGroundedAnswerParagraphInventoryBytes}, true, nil
	case WorkGroundedAnswerParagraphEvidenceRelation:
		maximum, err := opaqueModelChoiceBuilderResponseContract(
			groundedAnswerParagraphEvidenceRelationChoices,
		)
		return maximum, true, err
	case WorkGroundedParagraphRelevance:
		maximum, err := opaqueModelChoiceBuilderResponseContract(
			groundedParagraphRelevanceChoices,
		)
		return maximum, true, err
	default:
		return portableResponseContract{}, false, nil
	}
}
