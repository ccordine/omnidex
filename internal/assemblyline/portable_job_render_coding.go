package assemblyline

func renderPortableCodingJob(job PortableJob) (string, bool, error) {
	switch job.Kind {
	case WorkArtifactHandling:
		return handledPortableRender(renderDecodedPortableInput(job, BuildArtifactHandlingPrompt))
	case WorkCapabilityRelation:
		return handledPortableRender(renderDecodedPortableInput(job, BuildCapabilityRelationPrompt))
	case WorkRuntimeCapabilityNecessity:
		return handledPortableRender(renderDecodedPortableInput(
			job, BuildRuntimeCapabilityNecessityPrompt,
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
