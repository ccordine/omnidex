package assemblyline

func renderPortableRepositoryContextJob(job PortableJob) (string, bool, error) {
	switch job.Kind {
	case WorkContextRelevanceRelation:
		return handledPortableRender(renderDecodedPortableInput(job, BuildContextRelevanceRelationPrompt))
	case WorkContextMinification:
		return handledPortableRender(renderDecodedPortableInput(job, BuildContextMinificationPrompt))
	default:
		return "", false, nil
	}
}
