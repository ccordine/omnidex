package worker

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func replayExactStationSourceArtifact(
	job assemblyline.PortableJob,
	raw string,
	artifact ExactStationReplayArtifact,
) (ExactStationReplayArtifact, error) {
	if job.Kind == assemblyline.WorkTypeScriptRepairGuidance {
		guidance, err := assemblyline.DecodeTypeScriptRepairGuidanceResult(job, raw)
		artifact.Kind = "typescript_repair_guidance"
		if err != nil {
			return artifact, err
		}
		artifact.Source = guidance.Instruction
		artifact.SourceSHA256 = replaySHA256(guidance.Instruction)
		artifact.StartByte, artifact.EndByte = 0, len(guidance.Instruction)
		artifact.DiscardedBytes = len(raw) - len(guidance.Instruction)
		return artifact, nil
	}
	if job.Kind != assemblyline.WorkFragmentGeneration &&
		job.Kind != assemblyline.WorkFragmentModification &&
		job.Kind != assemblyline.WorkFragmentCorrection {
		return artifact, nil
	}
	var correction assemblyline.FragmentCorrectionInput
	var modification assemblyline.FragmentModificationInput
	var signature, language, current string
	var region *assemblyline.TypeScriptFragmentRepairRegion
	switch job.Kind {
	case assemblyline.WorkFragmentGeneration:
		var generation assemblyline.FragmentGenerationInput
		if err := json.Unmarshal(job.Payload, &generation); err != nil {
			return artifact, fmt.Errorf("decode replay fragment generation input: %w", err)
		}
		signature, language = generation.Signature, generation.Language
	case assemblyline.WorkFragmentModification:
		if err := json.Unmarshal(job.Payload, &modification); err != nil {
			return artifact, fmt.Errorf("decode replay fragment modification input: %w", err)
		}
		signature, language, current = modification.Signature, modification.Language,
			modification.CurrentDeclaration
	case assemblyline.WorkFragmentCorrection:
		if err := json.Unmarshal(job.Payload, &correction); err != nil {
			return artifact, fmt.Errorf("decode replay fragment correction input: %w", err)
		}
		signature, language, current, region = correction.Signature, correction.Language,
			correction.CurrentDeclaration, correction.RepairRegion
		if language == "" {
			if job.SourceProjection == "" {
				return artifact, fmt.Errorf(
					"replay language-blind fragment correction requires one persisted source projection identity",
				)
			}
			language = job.SourceProjection
		}
	}
	if language != "typescript" {
		if language == "" {
			return artifact, nil
		}
		projection, err := projectDirectCodingSourceDeclaration(language, raw)
		if err != nil {
			return artifact, fmt.Errorf("project replay source declaration: %w", err)
		}
		artifact.Kind = string(projection.Kind)
		artifact.Source, artifact.SourceSHA256 = projection.Source, projection.SourceSHA256
		artifact.StartByte, artifact.EndByte = projection.StartByte, projection.EndByte
		artifact.DiscardedBytes = projection.DiscardedBytes
		artifact.ChangedFromBase = current != "" && projection.Source != current
		return artifact, nil
	}
	if region != nil {
		replacement, err := assemblyline.ProjectTypeScriptFragmentRepairResponse(*region, raw)
		artifact.Kind, artifact.Source = "typescript_repair_region", replacement
		artifact.SourceSHA256 = replaySHA256(replacement)
		artifact.StartByte, artifact.EndByte = 0, len(replacement)
		artifact.DiscardedBytes = len(raw) - len(replacement)
		artifact.ChangedFromBase = replacement != region.Source
		if err != nil {
			return artifact, fmt.Errorf("project replay TypeScript repair region: %w", err)
		}
		return artifact, nil
	}
	projection, err := assemblyline.ProjectTypeScriptFunctionModelResponse(
		assemblyline.TypeScriptFunctionContract{Signature: signature, TSX: true}, raw,
	)
	artifact.Kind, artifact.Source, artifact.SourceSHA256 = "typescript_function", projection.Source, projection.SourceSHA256
	artifact.StartByte, artifact.EndByte, artifact.DiscardedBytes = projection.StartByte, projection.EndByte, projection.DiscardedBytes
	artifact.ChangedFromBase = current != "" && projection.Source != current
	if err != nil {
		return artifact, fmt.Errorf("project replay TypeScript function: %w", err)
	}
	return artifact, nil
}
