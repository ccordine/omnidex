package worker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func (executor *directCodingLanguageProjectStageExecutor) verifyLanguageStageCommands(
	program *directCodingProgram,
	commands []testCommand,
) error {
	documents, err := composeDirectCodingSourceProgram(*program)
	if err != nil {
		return err
	}
	if err := executor.writeLanguageStage(*program); err != nil {
		return err
	}
	for commandIndex, command := range commands {
		if len(executor.config.CleanupCommands) != 0 {
			executor.cleanupRequired = true
		}
		if strings.TrimSpace(command.Name) == "" {
			return fmt.Errorf("staged %s command is empty", executor.config.Language)
		}
		output, commandErr := executor.runLanguageStageCommand(command)
		for commandErr != nil {
			if !executor.config.Repair.stageFailureEnabled() {
				return executor.unrepairableLanguageStageFailure(command, output, commandErr)
			}
			mapped, ok, mapErr := executor.config.Repair.MapStageFailure(
				*program, documents, command, output,
			)
			if mapErr != nil {
				return fmt.Errorf("map staged %s failure: %w", executor.config.Language, mapErr)
			}
			if !ok {
				return executor.unrepairableLanguageStageFailure(command, output, commandErr)
			}
			input, err := directCodingLanguageFragmentInput(
				program, mapped.Target, executor.config.Language,
			)
			if err != nil {
				return err
			}
			current := strings.TrimSpace(program.Generated[mapped.Target.Block.ID])
			candidate, err := executor.repairLanguageBlock(
				program, mapped.Target, input, current, mapped.Diagnostic,
				executor.config.ValidateFragment,
			)
			if err != nil {
				return fmt.Errorf("repair staged %s block %s: %w", executor.config.Language, mapped.Target.Block.ID, err)
			}
			if err := applyDirectCodingLanguageRepair(
				program, mapped.Target.Block.ID, current, candidate,
			); err != nil {
				return err
			}
			documents, err = composeDirectCodingSourceProgram(*program)
			if err != nil {
				return err
			}
			if err := executor.writeLanguageStage(*program); err != nil {
				return err
			}
			if err := executor.refreshLanguageStage(commands[:commandIndex]); err != nil {
				return err
			}
			output, commandErr = executor.runLanguageStageCommand(command)
		}
	}
	return nil
}

func (executor *directCodingLanguageProjectStageExecutor) refreshLanguageStage(
	completed []testCommand,
) error {
	for index := len(completed) - 1; index >= 0; index-- {
		if completed[index].Purpose != verificationBuild {
			continue
		}
		output, err := executor.runLanguageStageCommand(completed[index])
		if err != nil {
			return fmt.Errorf(
				"refresh staged %s build after source correction: %w\n%s",
				executor.config.Language, err, trimForBudget(output, 12_000),
			)
		}
		return nil
	}
	return nil
}

func (executor *directCodingLanguageProjectStageExecutor) writeLanguageStage(
	program directCodingProgram,
) error {
	if err := resetDirectCodingLanguageStage(executor.root); err != nil {
		return err
	}
	assembly, err := directCodingAssemblyFromProgram(program)
	if err != nil {
		return err
	}
	stack, err := directCodingProjectStackByID(program.StackID)
	if err != nil {
		return err
	}
	for _, file := range assembly.Files {
		if err := validateDirectCodingStackArtifactSource(stack, file.Path, []byte(file.Content)); err != nil {
			return fmt.Errorf("validate staged %s artifact %s: %w", executor.config.Language, file.Path, err)
		}
		target := filepath.Join(executor.root, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return fmt.Errorf("create staged %s directory for %s: %w", executor.config.Language, file.Path, err)
		}
		if err := os.WriteFile(target, []byte(file.Content), 0o600); err != nil {
			return fmt.Errorf("write staged %s source %s: %w", executor.config.Language, file.Path, err)
		}
	}
	return nil
}

func (executor *directCodingLanguageProjectStageExecutor) runLanguageStageCommand(
	command testCommand,
) (string, error) {
	timeout := executor.config.Timeout
	if command.Timeout > 0 {
		timeout = command.Timeout
	}
	return runDirectCodingStageCommand(
		executor.session.runtime.ctx, executor.root, timeout, command.Name, command.Args...,
	)
}

func (executor *directCodingLanguageProjectStageExecutor) unrepairableLanguageStageFailure(
	command testCommand,
	output string,
	commandErr error,
) error {
	return fmt.Errorf(
		"staged %s command %s %s failed without one repairable block-owned diagnostic: %w\n%s",
		executor.config.Language, command.Name, strings.Join(command.Args, " "),
		commandErr, trimForBudget(output, 12_000),
	)
}

func directCodingSourceBlockRef(
	blueprint assemblyline.SourceBlueprint,
	blockID string,
) (assemblyline.SourceBlockRef, bool) {
	for documentIndex, document := range blueprint.Documents {
		for blockIndex, block := range document.Blocks {
			if block.ID == blockID {
				return assemblyline.SourceBlockRef{
					DocumentIndex: documentIndex, BlockIndex: blockIndex,
					Document: document, Block: block,
				}, true
			}
		}
	}
	return assemblyline.SourceBlockRef{}, false
}
