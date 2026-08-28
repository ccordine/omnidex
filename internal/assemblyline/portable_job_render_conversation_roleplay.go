package assemblyline

func renderPortableConversationRoleplayJob(job PortableJob) (string, bool, error) {
	switch job.Kind {
	case WorkConversationObjectiveKind:
		return handledPortableRender(renderDecodedPortableInput(job, BuildConversationObjectiveKindPrompt))
	case WorkConversationResponse:
		return handledPortableRender(renderDecodedPortableInput(job, BuildConversationResponsePrompt))
	case WorkRoleplayGroundedResponseText:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRoleplayGroundedResponseTextPrompt))
	case WorkRoleplayGroundedResponseEvidenceRelation:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRoleplayGroundedResponseEvidenceRelationPrompt))
	case WorkRoleplayCanonFactCoverage:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRoleplayCanonFactCoveragePrompt))
	case WorkRoleplayCanonFact:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRoleplayCanonFactPrompt))
	case WorkRoleplayOngoingAction:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRoleplayOngoingActionPrompt))
	case WorkGroundedAnswerText:
		return handledPortableRender(renderDecodedPortableInput(job, BuildGroundedAnswerTextPrompt))
	case WorkGroundedAnswerEvidenceRelation:
		return handledPortableRender(renderDecodedPortableInput(job, BuildGroundedAnswerEvidenceRelationPrompt))
	default:
		return "", false, nil
	}
}
