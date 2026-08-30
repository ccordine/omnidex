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
	case WorkRoleplayCanonFactInventory:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRoleplayCanonFactInventoryPrompt))
	case WorkRoleplayCanonFactCandidateAuthorization:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRoleplayCanonFactCandidateAuthorizationPrompt))
	case WorkRoleplayCanonFactCandidateRelation:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRoleplayCanonFactCandidateRelationPrompt))
	case WorkRoleplayOngoingAction:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRoleplayOngoingActionPrompt))
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
