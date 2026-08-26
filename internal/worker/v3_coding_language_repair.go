package worker

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

const maxDirectCodingLanguageRepairDiagnosticBytes = 1024

func (executor *directCodingLanguageProjectStageExecutor) repairLanguageBlock(
	stage *directCodingProgram,
	ref assemblyline.SourceBlockRef,
	generation assemblyline.FragmentGenerationInput,
	current string,
	diagnostic string,
	validator directCodingLanguageFragmentValidator,
) (string, error) {
	if stage == nil || !executor.config.Repair.enabled() || !ref.Block.Generated() {
		return "", fmt.Errorf("language repair requires one configured generated block")
	}
	if ref.Block.Role == assemblyline.SourceBlockTaskVerification {
		return "", fmt.Errorf("generated verification source is not repair model context")
	}
	if executor.repairAttempts[ref.Block.ID] >= maxDirectCodingLanguageCorrections {
		return "", fmt.Errorf(
			"block %s exhausted its %d code-owned corrections",
			ref.Block.ID, maxDirectCodingLanguageCorrections,
		)
	}
	if strings.TrimSpace(current) == "" {
		return "", fmt.Errorf("block %s repair current source is empty", ref.Block.ID)
	}
	if executor.repairSources[ref.Block.ID] == nil {
		executor.repairSources[ref.Block.ID] = map[string]struct{}{current: {}}
	}
	capabilities, symbols, err := directCodingLanguageRepairContext(stage, ref)
	if err != nil {
		return "", err
	}
	guidanceModel, err := executor.session.workerModel(station.CodingFragmentRepairGuidance)
	if err != nil {
		return "", err
	}
	correctionModel, err := executor.session.workerModel(station.CodingFragmentCorrection)
	if err != nil {
		return "", err
	}
	var prior *assemblyline.FragmentRepairGuidanceRejection
	for attempt := 0; attempt < maxDirectCodingLanguageCorrections; attempt++ {
		input := assemblyline.FragmentRepairGuidanceInput{
			Language: generation.Language, Dialect: generation.Dialect,
			Signature:          generation.Signature,
			Capabilities:       capabilities,
			PermittedSymbols:   symbols,
			CurrentDeclaration: strings.TrimSpace(current),
			Diagnostic:         strings.TrimSpace(diagnostic),
			PriorRejection:     prior,
		}
		guidance, err := runDirectCodingLanguageRepairGuidance(
			directCodingWorkerRuntime(executor.session), guidanceModel, ref.Block.ID, input,
		)
		if err != nil {
			return "", err
		}
		guidance = strings.TrimSpace(guidance)
		if err := executor.acceptLanguageRepairGuidance(ref.Block.ID, guidance); err != nil {
			return "", err
		}
		candidate, err := runDirectCodingLanguageCorrection(
			directCodingWorkerRuntime(executor.session), correctionModel, ref.Block.ID,
			current, guidance,
			func(candidate string) (string, error) {
				return validator(generation, candidate)
			},
		)
		if err == nil {
			if err := executor.acceptLanguageRepairSource(ref.Block.ID, candidate); err != nil {
				return "", err
			}
			executor.repairAttempts[ref.Block.ID]++
			return candidate, nil
		}
		if !errors.Is(err, errDirectCodingLanguageCorrectionUnchanged) {
			return "", err
		}
		prior = &assemblyline.FragmentRepairGuidanceRejection{
			Instruction: guidance,
			Failure:     assemblyline.FragmentRepairGuidanceNoSourceChange,
		}
	}
	return "", fmt.Errorf(
		"block %s repair made no source transition after %d bounded attempts",
		ref.Block.ID, maxDirectCodingLanguageCorrections,
	)
}

func directCodingLanguageRepairContext(
	stage *directCodingProgram,
	ref assemblyline.SourceBlockRef,
) ([]string, []string, error) {
	blocks := make(map[string]assemblyline.SourceBlock)
	for _, document := range stage.Source.Documents {
		for _, block := range document.Blocks {
			blocks[block.ID] = block
		}
	}
	capabilities := make([]string, 0, len(ref.Block.Capabilities))
	for _, capabilityID := range ref.Block.Capabilities {
		capability, exists := blocks[capabilityID]
		if !exists || strings.TrimSpace(capability.API) == "" {
			return nil, nil, fmt.Errorf(
				"block %s lacks repair capability %s", ref.Block.ID, capabilityID,
			)
		}
		capabilities = append(capabilities, capability.API)
	}
	return capabilities, append([]string(nil), ref.Block.Globals...), nil
}

func (executor *directCodingLanguageProjectStageExecutor) acceptLanguageRepairGuidance(
	blockID string,
	guidance string,
) error {
	seen := executor.repairGuidance[blockID]
	if seen == nil {
		seen = make(map[string]struct{})
		executor.repairGuidance[blockID] = seen
	}
	if _, repeated := seen[guidance]; repeated {
		return fmt.Errorf("block %s: %w", blockID, errDirectCodingLanguageGuidanceRepeated)
	}
	seen[guidance] = struct{}{}
	return nil
}

func (executor *directCodingLanguageProjectStageExecutor) acceptLanguageRepairSource(
	blockID string,
	source string,
) error {
	seen := executor.repairSources[blockID]
	if seen == nil {
		seen = make(map[string]struct{})
		executor.repairSources[blockID] = seen
	}
	if _, repeated := seen[source]; repeated {
		return fmt.Errorf("block %s repair repeated an already accepted source state", blockID)
	}
	seen[source] = struct{}{}
	return nil
}

func applyDirectCodingLanguageRepair(
	program *directCodingProgram,
	blockID string,
	current string,
	candidate string,
) error {
	if program == nil || strings.TrimSpace(blockID) == "" || strings.TrimSpace(candidate) == "" {
		return fmt.Errorf("apply language repair requires one program, block, and candidate")
	}
	if program.Generated[blockID] != current {
		return fmt.Errorf("block %s changed after its repair authority was captured", blockID)
	}
	retained := make(map[string]string, len(program.Generated)-1)
	for retainedID, source := range program.Generated {
		if retainedID != blockID {
			retained[retainedID] = source
		}
	}
	program.Generated[blockID] = candidate
	for retainedID, source := range retained {
		if program.Generated[retainedID] != source {
			return fmt.Errorf("repair of block %s changed accepted block %s", blockID, retainedID)
		}
	}
	if len(program.Generated) != len(retained)+1 {
		return fmt.Errorf("repair of block %s changed the generated-source set", blockID)
	}
	return nil
}

func directCodingLanguageParserRepairDiagnostic(
	session *directCodingSession,
	failure error,
) (string, error) {
	if session == nil || failure == nil {
		return "", fmt.Errorf("parser repair requires one exact failure")
	}
	diagnostic := trimForBudget(
		strings.TrimSpace(failure.Error()), maxDirectCodingLanguageRepairDiagnosticBytes,
	)
	if diagnostic == "" {
		return "", fmt.Errorf("parser repair diagnostic is empty")
	}
	if err := assemblyline.ValidatePathFreeModelContextWithProvenance(
		"language parser repair diagnostic",
		directCodingWorkerRuntime(session).PathProvenance,
		diagnostic,
	); err != nil {
		return "", err
	}
	return "SOURCE_DIAGNOSTIC: " + diagnostic, nil
}
