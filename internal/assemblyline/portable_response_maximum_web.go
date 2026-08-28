package assemblyline

func portableWebResponseMaximum(job PortableJob) (int, bool, error) {
	switch job.Kind {
	case WorkWebSearchTermCoverage:
		return maximumStringBytes(WebQueryTermRemains, WebNoUncoveredQueryTerm), true, nil
	case WorkWebSearchTerm:
		var input WebSearchTermLeafInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return 0, true, err
		}
		return input.MaxTermBytes, true, nil
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
	case WorkWebGroundedSynthesisCorrection:
		var input WebGroundedSynthesisCorrectionInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return 0, true, err
		}
		return input.MaxParagraphBytes, true, nil
	case WorkWebReviewClaimCoverage:
		return maximumStringBytes(
			WebReviewClaimRemains, WebReviewNoUncoveredClaim,
		), true, nil
	case WorkWebReviewClaim:
		return maxWebReviewClaimBytes, true, nil
	case WorkWebReviewClaimVerdict:
		return maximumStringBytes(
			WebReviewClaimSupported, WebReviewClaimInsufficient,
			WebReviewClaimContradicted, WebReviewClaimMismatch,
		), true, nil
	case WorkWebReviewIssueEvidenceRelation:
		return maximumStringBytes(
			WebReviewEvidenceImplicated, WebReviewEvidenceNotImplicated,
		), true, nil
	case WorkWebReviewIssueDetail:
		return maxWebReviewIssueDetailBytes, true, nil
	default:
		return 0, false, nil
	}
}
