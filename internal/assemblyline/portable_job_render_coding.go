package assemblyline

func renderPortableCodingJob(job PortableJob) (string, bool, error) {
	switch job.Kind {
	case WorkArtifactHandling:
		return handledPortableRender(renderDecodedPortableInput(job, BuildArtifactHandlingPrompt))
	case WorkKnownArtifactTruth:
		return handledPortableRender(renderDecodedPortableInput(job, BuildKnownArtifactTruthPrompt))
	case WorkDeclarationArtifactBoundary:
		return handledPortableRender(renderDecodedPortableInput(job, BuildDeclarationArtifactBoundaryPrompt))
	case WorkArtifactCandidateSelection:
		return handledPortableRender(renderDecodedPortableInput(job, BuildArtifactCandidateSelectionPrompt))
	case WorkCapabilityRelation:
		return handledPortableRender(renderDecodedPortableInput(job, BuildCapabilityRelationPrompt))
	case WorkSkillSelection:
		return handledPortableRender(renderDecodedPortableInput(job, BuildSkillSelectionPrompt))
	case WorkTypeScriptRepairGuidance:
		return handledPortableRender(renderDecodedPortableInput(job, BuildFragmentRepairGuidancePrompt))
	case WorkFragmentGeneration:
		return handledPortableRender(renderDecodedPortableInput(job, renderPortableFragmentGeneration))
	case WorkFragmentModification:
		return handledPortableRender(renderDecodedPortableInput(job, BuildGoFragmentModificationPrompt))
	case WorkFragmentCorrection:
		return handledPortableRender(renderDecodedPortableInput(job, renderPortableFragmentCorrection))
	case WorkResponseCorrection:
		return handledPortableRender(renderDecodedPortableInput(job, renderPortableResponseCorrection))
	default:
		return "", false, nil
	}
}
