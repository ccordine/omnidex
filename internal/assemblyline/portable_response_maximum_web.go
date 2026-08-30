package assemblyline

func portableWebResponseMaximum(job PortableJob) (int, bool, error) {
	switch job.Kind {
	case WorkWebRelevanceRelation:
		return maximumStringBytes(
			WebCandidateRelevant, WebCandidateNotRelevant,
		), true, nil
	case WorkWebSynthesisParagraphInventory:
		var input WebGroundedSynthesisInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return 0, true, err
		}
		return webSynthesisParagraphInventoryMaximum(input), true, nil
	case WorkWebSynthesisEvidenceRelation:
		return maximumStringBytes(
			WebEvidenceSupportsParagraph, WebEvidenceDoesNotSupport,
		), true, nil
	case WorkWebSynthesisParagraphAuthorization:
		return maximumStringBytes(
			WebParagraphResponsiveAndFullySupported,
			WebParagraphNotResponsiveOrUnsupported,
		), true, nil
	default:
		return 0, false, nil
	}
}
