package worker

import "github.com/gryph/omnidex/internal/assemblyline"

func replayWebSemanticLeaf(
	job assemblyline.PortableJob,
	raw string,
) (bool, error) {
	switch job.Kind {
	case assemblyline.WorkWebSearchTermCoverage:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeWebSearchTermCoverageLeaf)
	case assemblyline.WorkWebSearchTerm:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeWebSearchTermLeaf)
	case assemblyline.WorkWebRelevanceRelation:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeWebRelevanceRelationLeaf)
	case assemblyline.WorkWebSynthesisParagraphCoverage:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeWebSynthesisParagraphCoverageDecision)
	case assemblyline.WorkWebSynthesisParagraph:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeWebSynthesisParagraphDecision)
	case assemblyline.WorkWebSynthesisEvidenceRelation:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeWebSynthesisEvidenceRelationDecision)
	case assemblyline.WorkWebGroundedSynthesisCorrection:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeWebGroundedSynthesisCorrectionDecision)
	case assemblyline.WorkWebReviewClaimCoverage:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeWebReviewClaimCoverageDecision)
	case assemblyline.WorkWebReviewClaim:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeWebReviewClaimDecision)
	case assemblyline.WorkWebReviewClaimVerdict:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeWebReviewClaimVerdictDecision)
	case assemblyline.WorkWebReviewIssueEvidenceRelation:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeWebReviewIssueEvidenceRelationDecision)
	case assemblyline.WorkWebReviewIssueDetail:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeWebReviewIssueDetailDecision)
	default:
		return false, nil
	}
}
