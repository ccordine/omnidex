package assemblyline

func portableWebResponseMaximum(job PortableJob) (int, bool, error) {
	switch job.Kind {
	case WorkWebRelevanceRelation:
		maximum, err := opaqueModelChoiceBuilderResponseMaximum(webRelevanceRelationChoices)
		return maximum, true, err
	default:
		return 0, false, nil
	}
}
