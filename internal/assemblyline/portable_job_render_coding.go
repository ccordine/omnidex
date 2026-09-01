package assemblyline

func renderPortableCodingJob(job PortableJob) (string, bool, error) {
	switch job.Kind {
	case WorkArtifactHandling:
		return handledPortableRender(renderDecodedPortableInput(job, BuildArtifactHandlingPrompt))
	case WorkCapabilityRelation:
		return handledPortableRender(renderDecodedPortableInput(job, BuildCapabilityRelationPrompt))
	case WorkFragmentGeneration:
		return handledPortableRender(renderDecodedPortableInput(job, renderPortableFragmentGeneration))
	default:
		return "", false, nil
	}
}
