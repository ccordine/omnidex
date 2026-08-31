package assemblyline

func renderPortableRepositoryContextJob(job PortableJob) (string, bool, error) {
	switch job.Kind {
	case WorkRepositoryRequirementInventory:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRepositoryRequirementInventoryPrompt))
	case WorkRepositoryRequirementCandidateAuthorization:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRepositoryRequirementCandidateAuthorizationPrompt))
	case WorkRepositoryRequirementCandidateRelation:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRepositoryRequirementCandidateRelationPrompt))
	case WorkRepositoryEvidenceRelevanceRelation:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRepositoryEvidenceRelevanceRelationPrompt))
	case WorkContextRelevanceRelation:
		return handledPortableRender(renderDecodedPortableInput(job, BuildContextRelevanceRelationPrompt))
	case WorkContextMinification:
		return handledPortableRender(renderDecodedPortableInput(job, BuildContextMinificationPrompt))
	default:
		return "", false, nil
	}
}
