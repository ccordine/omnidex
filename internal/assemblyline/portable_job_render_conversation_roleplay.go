package assemblyline

func renderPortableConversationRoleplayJob(job PortableJob) (string, bool, error) {
	switch job.Kind {
	case WorkConversationObjectiveKind:
		return handledPortableRender(renderDecodedPortableInput(job, BuildConversationObjectiveKindPrompt))
	case WorkConversationResponse:
		return handledPortableRender(renderDecodedPortableInput(job, BuildConversationResponsePrompt))
	case WorkRoleplayGroundedResponseParagraphInventory:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRoleplayGroundedParagraphInventoryPrompt))
	case WorkRoleplayGroundedResponseEvidenceRelation:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRoleplayGroundedResponseEvidenceRelationPrompt))
	case WorkRoleplayGroundedResponseParagraphAuthorization:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRoleplayGroundedParagraphAuthorizationPrompt))
	case WorkRoleplayCanonFactPresence:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRoleplayCanonFactPresencePrompt))
	case WorkRoleplayCanonFactInventory:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRoleplayCanonFactInventoryPrompt))
	case WorkRoleplayCanonFactCandidateAuthorization:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRoleplayCanonFactCandidateAuthorizationPrompt))
	case WorkRoleplayCanonFactCandidateRelation:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRoleplayCanonFactCandidateRelationPrompt))
	case WorkRoleplayOngoingActionRelation:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRoleplayOngoingActionRelationPrompt))
	case WorkRoleplayOngoingActionValue:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRoleplayOngoingActionValuePrompt))
	case WorkGroundedAnswerParagraphInventory:
		return handledPortableRender(renderDecodedPortableInput(job, BuildGroundedAnswerParagraphInventoryPrompt))
	case WorkGroundedAnswerParagraphEvidenceRelation:
		return handledPortableRender(renderDecodedPortableInput(job, BuildGroundedAnswerParagraphEvidenceRelationPrompt))
	case WorkGroundedAnswerParagraphAuthorization:
		return handledPortableRender(renderDecodedPortableInput(job, BuildGroundedAnswerParagraphAuthorizationPrompt))
	default:
		return "", false, nil
	}
}
