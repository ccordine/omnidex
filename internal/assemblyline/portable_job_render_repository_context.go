package assemblyline

func renderPortableRepositoryContextJob(job PortableJob) (string, bool, error) {
	switch job.Kind {
	case WorkRepositoryRequirementCoverage:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRepositoryRequirementCoveragePrompt))
	case WorkRepositoryRequirement:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRepositoryRequirementPrompt))
	case WorkRepositoryChangeOwner:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRepositoryChangeOwnerPrompt))
	case WorkRepositoryEvidenceRelevanceLeaf:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRepositoryEvidenceRelevanceLeafPrompt))
	case WorkContextRelevanceSelection:
		return handledPortableRender(renderDecodedPortableInput(job, BuildContextRelevanceSelectionPrompt))
	case WorkContextMinification:
		return handledPortableRender(renderDecodedPortableInput(job, BuildContextMinificationPrompt))
	default:
		return "", false, nil
	}
}
