package worker

import (
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
	guidanceModel, correctionModel, err := executor.languageRepairModels()
	if err != nil {
		return "", err
	}
	return executor.repairLanguageBlockWithRuntime(
		directCodingWorkerRuntime(executor.session), guidanceModel, correctionModel,
		stage, ref, generation, current, diagnostic, validator,
	)
}

func (executor *directCodingLanguageProjectStageExecutor) languageRepairModels() (
	string,
	string,
	error,
) {
	if executor == nil || executor.session == nil {
		return "", "", fmt.Errorf("language repair requires one active coding session")
	}
	guidanceModel, err := executor.session.workerModel(station.CodingFragmentRepairGuidance)
	if err != nil {
		return "", "", err
	}
	correctionModel, err := executor.session.workerModel(station.CodingFragmentCorrection)
	if err != nil {
		return "", "", err
	}
	return guidanceModel, correctionModel, nil
}

func (executor *directCodingLanguageProjectStageExecutor) repairLanguageBlockWithRuntime(
	runtime typedWorkerRuntime,
	guidanceModel string,
	correctionModel string,
	stage *directCodingProgram,
	ref assemblyline.SourceBlockRef,
	generation assemblyline.FragmentGenerationInput,
	current string,
	diagnostic string,
	validator directCodingLanguageFragmentValidator,
) (string, error) {
	if executor == nil || stage == nil || !ref.Block.Generated() || validator == nil {
		return "", fmt.Errorf("language repair requires one generated block and parser")
	}
	if ref.Block.Role == assemblyline.SourceBlockTaskVerification {
		return "", fmt.Errorf("generated verification source is not repair model context")
	}
	if executor.acceptedRepairTransitions == nil || executor.repairDiagnostics == nil {
		return "", fmt.Errorf("language repair state is not initialized")
	}
	if executor.acceptedRepairTransitions[ref.Block.ID] >= maxDirectCodingLanguageRepairTransitions {
		return "", fmt.Errorf(
			"block %s exhausted its %d accepted code-owned repair transitions",
			ref.Block.ID, maxDirectCodingLanguageRepairTransitions,
		)
	}
	if strings.TrimSpace(current) == "" {
		return "", fmt.Errorf("block %s repair current source is empty", ref.Block.ID)
	}
	diagnostic = strings.TrimSpace(diagnostic)
	if err := executor.observeLanguageRepairDiagnostic(ref.Block.ID, diagnostic); err != nil {
		return "", err
	}
	capabilities, symbols, err := directCodingLanguageRepairContext(stage, ref)
	if err != nil {
		return "", err
	}
	runtime.MaxAttempts = 1
	input := assemblyline.FragmentRepairGuidanceInput{
		Language: generation.Language, Dialect: generation.Dialect,
		Signature:          generation.Signature,
		Capabilities:       capabilities,
		PermittedSymbols:   symbols,
		CurrentDeclaration: strings.TrimSpace(current),
		Diagnostic:         diagnostic,
	}
	guidance, err := runDirectCodingLanguageRepairGuidance(
		runtime, guidanceModel, ref.Block.ID, input,
	)
	if err != nil {
		return "", err
	}
	candidate, err := runDirectCodingLanguageCorrection(
		runtime, correctionModel, ref.Block.ID,
		current, strings.TrimSpace(guidance), generation.Language,
		func(candidate string) (string, error) {
			return validator(generation, candidate)
		},
	)
	if err != nil {
		return "", fmt.Errorf(
			"block %s repair produced no accepted source transition: %w",
			ref.Block.ID, err,
		)
	}
	executor.acceptedRepairTransitions[ref.Block.ID]++
	return candidate, nil
}

func (executor *directCodingLanguageProjectStageExecutor) observeLanguageRepairDiagnostic(
	blockID string,
	diagnostic string,
) error {
	blockID = strings.TrimSpace(blockID)
	diagnostic = strings.TrimSpace(diagnostic)
	if blockID == "" || diagnostic == "" {
		return fmt.Errorf("language repair progress requires one block and exact diagnostic")
	}
	seen := executor.repairDiagnostics[blockID]
	if seen == nil {
		seen = make(map[string]struct{})
		executor.repairDiagnostics[blockID] = seen
	}
	if _, repeated := seen[diagnostic]; repeated {
		return fmt.Errorf(
			"block %s repeated a compiler diagnostic; no distinct verified failure authorizes another repair call: %s",
			blockID, safeLine(firstDirectCodingDiagnosticLine(diagnostic), "unknown"),
		)
	}
	seen[diagnostic] = struct{}{}
	return nil
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
	provenance assemblyline.ArtifactIdentityProvenance,
	failure error,
) (string, error) {
	if failure == nil {
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
		provenance,
		diagnostic,
	); err != nil {
		return "", err
	}
	return "SOURCE_DIAGNOSTIC: " + diagnostic, nil
}
