package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func runDirectCodingTypeScriptRepairGuidance(
	runtime typedWorkerRuntime,
	modelName string,
	block assemblyline.TypeScriptBlock,
	available string,
	current string,
	repairRegion *assemblyline.TypeScriptFragmentRepairRegion,
	diagnostic string,
) (string, error) {
	return runDirectCodingTypeScriptRepairGuidanceWithRejection(
		runtime, modelName, block, available, current, repairRegion, diagnostic, nil,
	)
}

func runDirectCodingTypeScriptRepairGuidanceAfterRejection(
	runtime typedWorkerRuntime,
	modelName string,
	block assemblyline.TypeScriptBlock,
	available string,
	current string,
	repairRegion *assemblyline.TypeScriptFragmentRepairRegion,
	diagnostic string,
	rejectedInstruction string,
	rejectionKind assemblyline.TypeScriptRepairGuidanceRejectionKind,
) (string, error) {
	return runDirectCodingTypeScriptRepairGuidanceWithRejection(
		runtime, modelName, block, available, current, repairRegion, diagnostic,
		&assemblyline.TypeScriptRepairGuidanceRejection{
			Instruction: strings.TrimSpace(rejectedInstruction),
			Failure:     rejectionKind,
		},
	)
}

func runDirectCodingTypeScriptRepairGuidanceWithRejection(
	runtime typedWorkerRuntime,
	modelName string,
	block assemblyline.TypeScriptBlock,
	available string,
	current string,
	repairRegion *assemblyline.TypeScriptFragmentRepairRegion,
	diagnostic string,
	priorRejection *assemblyline.TypeScriptRepairGuidanceRejection,
) (string, error) {
	capabilities := make([]string, 0, 1)
	if declaration := strings.TrimSpace(available); declaration != "" {
		capabilities = append(capabilities, declaration)
	}
	portableCurrent := strings.TrimSpace(current)
	if repairRegion != nil {
		portableCurrent = ""
	}
	job, err := assemblyline.NewTypeScriptRepairGuidanceJob(
		assemblyline.TypeScriptRepairGuidanceInput{
			Language: "typescript", Signature: strings.TrimSpace(block.Signature),
			Capabilities: capabilities, PermittedSymbols: append([]string(nil), block.Globals...),
			CurrentDeclaration: portableCurrent, RepairRegion: repairRegion,
			Diagnostic:     strings.TrimSpace(diagnostic),
			PriorRejection: priorRejection,
		},
	)
	if err != nil {
		return "", fmt.Errorf("construct TypeScript repair-guidance job: %w", err)
	}
	guidance, err := runDirectCodingSemanticCall[assemblyline.TypeScriptRepairGuidance](
		runtime, modelName, block.ID+":repair_guidance", job, nil,
		func(candidate assemblyline.TypeScriptRepairGuidance) error {
			return candidate.Validate()
		},
	)
	if err != nil {
		return "", err
	}
	return guidance.Instruction, nil
}
