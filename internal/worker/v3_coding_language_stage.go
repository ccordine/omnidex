package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

type directCodingLanguageStageConfig struct {
	Language           string
	AdapterID          string
	Timeout            time.Duration
	ValidateFragment   directCodingLanguageFragmentValidator
	ValidateAcceptance func(*directCodingProgram, assemblyline.SourceBlockRef, string) error
	TaskCommands       func(
		assemblyline.ApplicationTaskContext,
		directCodingProgram,
	) ([]testCommand, error)
	FinalCommands   func(directCodingProgram) ([]testCommand, error)
	CleanupCommands []testCommand
	Repair          directCodingLanguageRepairConfig
}

type directCodingLanguageProjectStageExecutor struct {
	session         *directCodingSession
	root            string
	removeEmptyRoot string
	config          directCodingLanguageStageConfig
	cleanupRequired bool
	repairAttempts  map[string]int
	repairGuidance  map[string]map[string]struct{}
	repairSources   map[string]map[string]struct{}
}

func newDirectCodingLanguageProjectStageExecutor(
	session *directCodingSession,
	config directCodingLanguageStageConfig,
) (directCodingProjectStageExecutor, error) {
	if session == nil {
		return nil, fmt.Errorf("%s project stage requires one coding session", config.Language)
	}
	if err := validateDirectCodingLanguageStageConfig(config); err != nil {
		return nil, err
	}
	root, removeEmptyRoot, err := createDirectCodingLanguageStageRoot(session, config)
	if err != nil {
		return nil, fmt.Errorf("create isolated %s coding stage: %w", config.Language, err)
	}
	return &directCodingLanguageProjectStageExecutor{
		session: session, root: root, removeEmptyRoot: removeEmptyRoot, config: config,
		repairAttempts: make(map[string]int), repairGuidance: make(map[string]map[string]struct{}),
		repairSources: make(map[string]map[string]struct{}),
	}, nil
}

func validateDirectCodingLanguageStageConfig(config directCodingLanguageStageConfig) error {
	if strings.TrimSpace(config.Language) == "" || strings.TrimSpace(config.AdapterID) == "" ||
		config.Timeout <= 0 || config.ValidateFragment == nil || config.TaskCommands == nil ||
		config.FinalCommands == nil {
		return fmt.Errorf("language stage configuration requires identity, parsers, timeout, and verification commands")
	}
	for index, command := range config.CleanupCommands {
		if err := validateV3Command(command.Name, command.Args); err != nil {
			return fmt.Errorf(
				"language stage cleanup command %d is outside the code-owned boundary: %w", index, err,
			)
		}
		if command.Timeout < 0 || command.Timeout > maxV3CommandLimit {
			return fmt.Errorf("language stage cleanup command %d has invalid timeout %s", index, command.Timeout)
		}
	}
	return nil
}

func (executor *directCodingLanguageProjectStageExecutor) GenerateBlock(
	_ assemblyline.ApplicationTaskContext,
	stage *directCodingProgram,
	ref assemblyline.SourceBlockRef,
) (string, error) {
	if ref.Document.AdapterID != executor.config.AdapterID {
		return "", fmt.Errorf(
			"%s project stage cannot generate adapter %q block %s",
			executor.config.Language, ref.Document.AdapterID, ref.Block.ID,
		)
	}
	input, err := directCodingLanguageFragmentInput(stage, ref, executor.config.Language)
	if err != nil {
		return "", err
	}
	modelName, err := executor.session.workerModel(station.CodingFragment)
	if err != nil {
		return "", err
	}
	runtime := directCodingWorkerRuntime(executor.session)
	runtime.MaxAttempts = 1
	runtime.CorrectionModel = ""
	validate := executor.config.ValidateFragment
	if ref.Block.Role == assemblyline.SourceBlockTaskVerification &&
		executor.config.ValidateAcceptance != nil {
		validate = func(
			input assemblyline.FragmentGenerationInput,
			candidate string,
		) (string, error) {
			source, err := executor.config.ValidateFragment(input, candidate)
			if err != nil {
				return "", err
			}
			if err := executor.config.ValidateAcceptance(stage, ref, source); err != nil {
				return "", err
			}
			return source, nil
		}
	}
	source, err := runDirectCodingLanguageFragmentWorker(
		runtime, modelName,
		directCodingLanguageGenerationJob{
			Subject: ref.Block.ID, Input: input, Validate: validate,
		},
	)
	if err != nil {
		var rejection *directCodingLanguageFragmentRejection
		if executor.config.Repair.enabled() &&
			ref.Block.Role != assemblyline.SourceBlockTaskVerification &&
			errors.As(err, &rejection) {
			diagnostic, diagnosticErr := directCodingLanguageParserRepairDiagnostic(
				executor.session, rejection.Failure,
			)
			if diagnosticErr != nil {
				return "", errors.Join(err, diagnosticErr)
			}
			return executor.repairLanguageBlock(
				stage, ref, input, rejection.Candidate, diagnostic, validate,
			)
		}
		return "", err
	}
	return source, nil
}

func (executor *directCodingLanguageProjectStageExecutor) VerifyTask(
	context assemblyline.ApplicationTaskContext,
	stage *directCodingProgram,
) error {
	commands, err := executor.config.TaskCommands(context, *stage)
	if err != nil {
		return err
	}
	return executor.verifyStagedAssembly(stage, commands)
}

func (executor *directCodingLanguageProjectStageExecutor) VerifyFinal(
	program *directCodingProgram,
) error {
	commands, err := executor.config.FinalCommands(*program)
	if err != nil {
		return err
	}
	return executor.verifyStagedAssembly(program, commands)
}

func (executor *directCodingLanguageProjectStageExecutor) verifyStagedAssembly(
	program *directCodingProgram,
	commands []testCommand,
) error {
	if program == nil || executor.root == "" || !filepath.IsAbs(executor.root) {
		return fmt.Errorf("staged %s verification requires a program and absolute temporary root", executor.config.Language)
	}
	if len(commands) == 0 {
		return fmt.Errorf("staged %s verification requires at least one command", executor.config.Language)
	}
	return executor.verifyLanguageStageCommands(program, commands)
}

func (executor *directCodingLanguageProjectStageExecutor) Close() error {
	if executor == nil || executor.root == "" {
		return nil
	}
	var cleanupErr error
	if executor.cleanupRequired {
		for _, command := range executor.config.CleanupCommands {
			timeout := command.Timeout
			if timeout == 0 {
				timeout = executor.config.Timeout
			}
			output, err := runDirectCodingStageCommand(
				context.Background(), executor.root, timeout,
				command.Name, command.Args...,
			)
			if err != nil {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf(
					"clean staged %s command %s %s: %w\n%s",
					executor.config.Language, command.Name, strings.Join(command.Args, " "),
					err, trimForBudget(output, 12_000),
				))
			}
		}
	}
	removeErr := os.RemoveAll(executor.root)
	if removeErr == nil && executor.removeEmptyRoot != "" {
		if err := os.Remove(executor.removeEmptyRoot); err != nil && !os.IsNotExist(err) {
			removeErr = fmt.Errorf("remove empty Docker-backed stage boundary: %w", err)
		}
	}
	executor.root = ""
	executor.removeEmptyRoot = ""
	if removeErr != nil {
		removeErr = fmt.Errorf("remove isolated %s stage: %w", executor.config.Language, removeErr)
	}
	return errors.Join(cleanupErr, removeErr)
}

func resetDirectCodingLanguageStage(root string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return fmt.Errorf("read isolated language stage: %w", err)
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
			return fmt.Errorf("reset isolated language stage entry %s: %w", entry.Name(), err)
		}
	}
	return nil
}
