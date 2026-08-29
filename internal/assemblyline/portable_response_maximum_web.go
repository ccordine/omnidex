package assemblyline

func portableWebResponseMaximum(job PortableJob) (int, bool, error) {
	switch job.Kind {
	case WorkWebRelevanceRelation:
		return maximumStringBytes(
			WebCandidateRelevant, WebCandidateNotRelevant,
		), true, nil
	case WorkWebSynthesisParagraphCoverage:
		return maximumStringBytes(
			WebSynthesisParagraphRemains, WebSynthesisNoUncoveredParagraph,
		), true, nil
	case WorkWebSynthesisParagraph:
		var input WebSynthesisParagraphLeafInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return 0, true, err
		}
		return input.MaxParagraphBytes, true, nil
	case WorkWebSynthesisEvidenceRelation:
		return maximumStringBytes(
			WebEvidenceSupportsParagraph, WebEvidenceDoesNotSupport,
		), true, nil
	default:
		return 0, false, nil
	}
}
