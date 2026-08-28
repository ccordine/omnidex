package assemblyline

func renderPortableRepositoryContextJob(job PortableJob) (string, bool, error) {
	switch job.Kind {
	case WorkRepositoryRequirementCoverage:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRepositoryRequirementCoveragePrompt))
	case WorkRepositoryRequirement:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRepositoryRequirementPrompt))
	case WorkRepositorySearchAnchorCoverage:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRepositorySearchAnchorCoveragePrompt))
	case WorkRepositorySearchAnchor:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRepositorySearchAnchorPrompt))
	case WorkRepositoryChangeOwner:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRepositoryChangeOwnerPrompt))
	case WorkRepositoryEvidenceRelevanceLeaf:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRepositoryEvidenceRelevanceLeafPrompt))
	case WorkRepositoryGroundedIssueDetail:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRepositoryGroundedIssueDetailPrompt))
	case WorkRepositoryGroundedIssueKind:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRepositoryGroundedIssueKindPrompt))
	case WorkRepositoryGroundedCorrection:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRepositoryGroundedCorrectionPrompt))
	case WorkContextSearchTermCoverage:
		return handledPortableRender(renderDecodedPortableInput(job, BuildContextSearchTermCoveragePrompt))
	case WorkContextSearchTerm:
		return handledPortableRender(renderDecodedPortableInput(job, BuildContextSearchTermPrompt))
	case WorkContextRelevanceSelection:
		return handledPortableRender(renderDecodedPortableInput(job, BuildContextRelevanceSelectionPrompt))
	case WorkContextMinification:
		return handledPortableRender(renderDecodedPortableInput(job, BuildContextMinificationPrompt))
	default:
		return "", false, nil
	}
}
