package assemblyline

func portableWebResponseMaximum(job PortableJob) (portableResponseContract, bool, error) {
	switch job.Kind {
	case WorkWebRelevanceRelation:
		maximum, err := opaqueModelChoiceBuilderResponseContract(webRelevanceRelationChoices)
		return maximum, true, err
	default:
		return portableResponseContract{}, false, nil
	}
}
