package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func runDirectCodingTypeScriptRepairGuidance(
	runtime typedWorkerRuntime,
	modelName string,
	block assemblyline.SourceBlock,
	dialect string,
	available string,
	current string,
	diagnostic string,
) (string, error) {
	capabilities := make([]string, 0, 1)
	if declaration := strings.TrimSpace(available); declaration != "" {
		capabilities = append(capabilities, declaration)
	}
	permittedSymbols := append([]string(nil), block.Globals...)
	job, err := assemblyline.NewFragmentRepairGuidanceJob(
		assemblyline.FragmentRepairGuidanceInput{
			Language: "typescript", Dialect: strings.TrimSpace(dialect),
			Signature:    strings.TrimSpace(block.Signature),
			Capabilities: capabilities, PermittedSymbols: permittedSymbols,
			CurrentDeclaration: strings.TrimSpace(current),
			Diagnostic: strings.TrimSpace(diagnostic),
		},
	)
	if err != nil {
		return "", fmt.Errorf("construct TypeScript repair-guidance job: %w", err)
	}
	guidance, err := runDirectCodingSemanticLeafCall(
		runtime, modelName, block.ID+":repair_guidance", job, nil,
		func(raw string) (assemblyline.FragmentRepairGuidance, error) {
			return assemblyline.DecodeFragmentRepairGuidanceResult(job, raw)
		},
		func(candidate assemblyline.FragmentRepairGuidance) error {
			return candidate.Validate()
		},
	)
	if err != nil {
		return "", err
	}
	return guidance.Instruction, nil
}
