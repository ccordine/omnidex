package assemblyline

func renderPortableCodingJob(job PortableJob) (string, bool, error) {
	switch job.Kind {
	case WorkArtifactHandling:
		return handledPortableRender(renderDecodedPortableInput(job, BuildArtifactHandlingPrompt))
	case WorkRepositoryArtifactAbsence:
		return handledPortableRender(renderDecodedPortableInput(job, BuildRepositoryArtifactAbsencePrompt))
	case WorkPlainTextArtifactCreation:
		return handledPortableRender(renderDecodedPortableInput(job, BuildPlainTextArtifactCreationPrompt))
	case WorkDeclarationArtifactBoundary:
		return handledPortableRender(renderDecodedPortableInput(job, BuildDeclarationArtifactBoundaryPrompt))
	case WorkArtifactCandidateSelection:
		return handledPortableRender(renderDecodedPortableInput(job, BuildArtifactCandidateSelectionPrompt))
	case WorkCapabilityRelation:
		return handledPortableRender(renderDecodedPortableInput(job, BuildCapabilityRelationPrompt))
	case WorkSkillSelection:
		return handledPortableRender(renderDecodedPortableInput(job, BuildSkillSelectionPrompt))
	case WorkRuntimeCapabilitySelection:
		return handledPortableRender(renderDecodedPortableInput(
			job, BuildRuntimeCapabilitySelectionPrompt,
		))
	case WorkTypeScriptRepairGuidance:
		return handledPortableRender(renderDecodedPortableInput(job, BuildFragmentRepairGuidancePrompt))
	case WorkFragmentGeneration:
		return handledPortableRender(renderDecodedPortableInput(job, renderPortableFragmentGeneration))
	case WorkFragmentGenerationReplacement:
		return handledPortableRender(renderDecodedPortableInput(
			job, renderPortableFragmentGenerationReplacement,
		))
	case WorkFragmentModification:
		return handledPortableRender(renderDecodedPortableInput(job, BuildGoFragmentModificationPrompt))
	case WorkFragmentCorrection:
		return handledPortableRender(renderDecodedPortableInput(job, renderPortableFragmentCorrection))
	default:
		return "", false, nil
	}
}
