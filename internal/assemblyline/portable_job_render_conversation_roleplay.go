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
	case WorkGroundedParagraphSupport:
		return handledPortableRender(renderDecodedPortableInput(job, BuildGroundedParagraphSupportPrompt))
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
	case WorkGroundedParagraphRelevance:
		return handledPortableRender(renderDecodedPortableInput(job, BuildGroundedParagraphRelevancePrompt))
	default:
		return "", false, nil
	}
}
