package worker

import "github.com/gryph/omnidex/internal/assemblyline"

func replayWebSemanticLeaf(
	job assemblyline.PortableJob,
	raw string,
) (bool, error) {
	switch job.Kind {
	case assemblyline.WorkWebRelevanceRelation:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeWebRelevanceRelationLeaf)
	case assemblyline.WorkWebSynthesisParagraphInventory:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeWebSynthesisParagraphInventory)
	case assemblyline.WorkWebSynthesisEvidenceRelation:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeWebSynthesisEvidenceRelationDecision)
	case assemblyline.WorkWebSynthesisParagraphAuthorization:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeWebSynthesisParagraphAuthorizationDecision)
	default:
		return false, nil
	}
}
