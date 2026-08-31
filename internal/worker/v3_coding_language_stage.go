package worker

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

type directCodingLanguageSourceConfig struct {
	Language         string
	AdapterID        string
	ValidateFragment directCodingLanguageFragmentValidator
}

type directCodingLanguageRepairModelResolver func() (string, string, error)

type directCodingLanguageSourceGenerator struct {
	session                   *directCodingSession
	config                    directCodingLanguageSourceConfig
	acceptedRepairTransitions map[string]int
	repairDiagnostics         map[string]map[string]struct{}
}

func newDirectCodingLanguageSourceGenerator(
	session *directCodingSession,
	config directCodingLanguageSourceConfig,
) (directCodingProjectSourceGenerator, error) {
	if session == nil {
		return nil, fmt.Errorf("%s source generation requires one coding session", config.Language)
	}
	if err := validateDirectCodingLanguageSourceConfig(config); err != nil {
		return nil, err
	}
	return &directCodingLanguageSourceGenerator{
		session: session, config: config,
		acceptedRepairTransitions: make(map[string]int),
		repairDiagnostics:         make(map[string]map[string]struct{}),
	}, nil
}

func validateDirectCodingLanguageSourceConfig(config directCodingLanguageSourceConfig) error {
	if strings.TrimSpace(config.Language) == "" || strings.TrimSpace(config.AdapterID) == "" ||
		config.ValidateFragment == nil {
		return fmt.Errorf("language source generation requires identity and a parser")
	}
	if _, err := directCodingSourceDeclarationProjector(config.Language); err != nil {
		return fmt.Errorf("language source configuration projection: %w", err)
	}
	return nil
}

func (executor *directCodingLanguageSourceGenerator) GenerateBlock(
	_ assemblyline.ApplicationTaskContext,
	stage *directCodingProgram,
	ref assemblyline.SourceBlockRef,
) (string, error) {
	if ref.Document.AdapterID != executor.config.AdapterID {
		return "", fmt.Errorf(
			"%s source generator cannot generate adapter %q block %s",
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
	if ref.Block.Role == assemblyline.SourceBlockTaskVerification {
		return "", fmt.Errorf("generated verification block %s is obsolete", ref.Block.ID)
	}
	return executor.generateBlockWithRuntime(
		runtime, modelName, executor.languageRepairModels,
		stage, ref, input, executor.config.ValidateFragment,
	)
}

func (executor *directCodingLanguageSourceGenerator) generateBlockWithRuntime(
	runtime typedWorkerRuntime,
	modelName string,
	repairModels directCodingLanguageRepairModelResolver,
	stage *directCodingProgram,
	ref assemblyline.SourceBlockRef,
	input assemblyline.FragmentGenerationInput,
	validate directCodingLanguageFragmentValidator,
) (string, error) {
	source, err := runDirectCodingLanguageFragmentWorker(
		runtime, modelName,
		directCodingLanguageGenerationJob{
			Subject: ref.Block.ID, Input: input,
			Project: func(raw string) (assemblyline.PortableResultProjection, error) {
				return projectDirectCodingSourceDeclaration(executor.config.Language, raw)
			},
			Validate: validate,
		},
	)
	if err != nil {
		var rejection *directCodingLanguageFragmentRejection
		if ref.Block.Role != assemblyline.SourceBlockTaskVerification &&
			errors.As(err, &rejection) {
			diagnostic, diagnosticErr := directCodingLanguageParserRepairDiagnostic(
				runtime.PathProvenance, rejection.Failure,
			)
			if diagnosticErr != nil {
				return "", errors.Join(err, diagnosticErr)
			}
			if repairModels == nil {
				return "", fmt.Errorf("initial language fragment repair model routing is unavailable")
			}
			guidanceModel, correctionModel, modelErr := repairModels()
			if modelErr != nil {
				return "", modelErr
			}
			repairRuntime := runtime
			repairRuntime.MaxAttempts = 1
			return executor.repairLanguageBlockWithRuntime(
				repairRuntime, guidanceModel, correctionModel,
				stage, ref, input, rejection.Candidate, diagnostic, validate,
			)
		}
		return "", err
	}
	return source, nil
}
