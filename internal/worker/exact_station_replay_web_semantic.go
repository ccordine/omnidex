package worker

import "github.com/gryph/omnidex/internal/assemblyline"

func replayWebSemanticLeaf(
	job assemblyline.PortableJob,
	raw string,
) (bool, error) {
	switch job.Kind {
	case assemblyline.WorkWebRelevanceRelation:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeWebRelevanceRelationLeaf)
	case assemblyline.WorkWebSynthesisParagraphCoverage:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeWebSynthesisParagraphCoverageDecision)
	case assemblyline.WorkWebSynthesisParagraph:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeWebSynthesisParagraphDecision)
	case assemblyline.WorkWebSynthesisEvidenceRelation:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeWebSynthesisEvidenceRelationDecision)
	default:
		return false, nil
	}
}
