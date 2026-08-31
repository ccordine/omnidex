package assemblyline

func portableWebResponseMaximum(job PortableJob) (int, bool, error) {
	switch job.Kind {
	case WorkWebRelevanceRelation:
		return maximumStringBytes(
			WebCandidateRelevant, WebCandidateNotRelevant,
		), true, nil
	default:
		return 0, false, nil
	}
}
