package worker

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func replayGeneralSemanticLeaf(
	job assemblyline.PortableJob,
	raw string,
) (bool, error) {
	switch job.Kind {
	case assemblyline.WorkArtifactHandling:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeArtifactHandlingDecision)
	case assemblyline.WorkKnownArtifactTruth:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeKnownArtifactTruthDecision)
	case assemblyline.WorkDeclarationArtifactBoundary:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeDeclarationArtifactBoundaryDecision)
	case assemblyline.WorkArtifactCandidateSelection:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeArtifactCandidateSelectionDecision)
	case assemblyline.WorkCapabilityRelation:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeCapabilityRelationDecision)
	case assemblyline.WorkSkillSelection:
		return true, decodeReplaySemanticLeaf(job, raw, assemblyline.DecodeSkillSelectionDecision)
	case assemblyline.WorkResponseCorrection:
		return true, replayResponseCorrectionSemanticLeaf(job, raw)
	default:
		return false, nil
	}
}

func replayResponseCorrectionSemanticLeaf(job assemblyline.PortableJob, raw string) error {
	replacement, err := assemblyline.DecodeResponseCorrectionReplacement(job, raw)
	if err != nil {
		return err
	}
	if replacement != raw {
		return fmt.Errorf("response correction altered exact raw replacement bytes")
	}
	var input assemblyline.ResponseCorrectionInput
	if err := json.Unmarshal(job.Payload, &input); err != nil {
		return fmt.Errorf("decode correction authority: %w", err)
	}
	if _, err := replayExactStationArtifact(input.Original, replacement); err != nil {
		return fmt.Errorf("validate corrected %s leaf: %w", input.Original.Kind, err)
	}
	return nil
}
