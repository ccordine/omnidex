package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

type directCodingLanguageSourceConfig struct {
	Language           string
	AdapterID          string
	ValidateFragment   directCodingLanguageFragmentValidator
	ValidateAcceptance func(assemblyline.SourceBlockRef, string) error
}

type directCodingLanguageSourceGenerator struct {
	session *directCodingSession
	config  directCodingLanguageSourceConfig
}

func newDirectCodingLanguageSourceGenerator(
	session *directCodingSession,
	config directCodingLanguageSourceConfig,
) (*directCodingLanguageSourceGenerator, error) {
	if session == nil {
		return nil, fmt.Errorf("%s source generation requires one coding session", config.Language)
	}
	if err := validateDirectCodingLanguageSourceConfig(config); err != nil {
		return nil, err
	}
	return &directCodingLanguageSourceGenerator{
		session: session, config: config,
	}, nil
}

func newDirectCodingLanguageSourceGeneratorForProgram(
	session *directCodingSession,
	program directCodingProgram,
) (*directCodingLanguageSourceGenerator, error) {
	var selected directCodingArtifactAdapter
	for _, document := range program.Source.Documents {
		hasGeneratedBlock := false
		for _, block := range document.Blocks {
			if block.Generated() {
				hasGeneratedBlock = true
				break
			}
		}
		if !hasGeneratedBlock {
			continue
		}
		adapter, err := directCodingArtifactAdapterByID(document.AdapterID)
		if err != nil {
			return nil, err
		}
		if adapter.SourceLanguage == "" || adapter.ValidateFragment == nil {
			return nil, fmt.Errorf(
				"artifact adapter %s cannot consume generated source blocks", adapter.ID,
			)
		}
		if selected.ID != "" && selected.ID != adapter.ID {
			return nil, fmt.Errorf(
				"one source generator cannot consume both %s and %s artifact adapters",
				selected.ID, adapter.ID,
			)
		}
		selected = adapter
	}
	if selected.ID == "" {
		return nil, fmt.Errorf("source generation requires one generated artifact adapter")
	}
	return newDirectCodingLanguageSourceGenerator(session, directCodingLanguageSourceConfig{
		Language:           selected.SourceLanguage,
		AdapterID:          selected.ID,
		ValidateFragment:   selected.ValidateFragment,
		ValidateAcceptance: selected.ValidateAcceptance,
	})
}

func validateDirectCodingLanguageSourceConfig(config directCodingLanguageSourceConfig) error {
	if strings.TrimSpace(config.Language) == "" || strings.TrimSpace(config.AdapterID) == "" ||
		config.ValidateFragment == nil {
		return fmt.Errorf("language source generation requires identity and parser")
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
	runtime.MaxAttempts = assemblyline.MaxSourceBodyAttempts
	return executor.generateBlockWithRuntime(runtime, modelName, ref, input)
}

func (executor *directCodingLanguageSourceGenerator) generateBlockWithRuntime(
	runtime typedWorkerRuntime,
	modelName string,
	ref assemblyline.SourceBlockRef,
	input assemblyline.FragmentGenerationInput,
) (string, error) {
	validate := executor.config.ValidateFragment
	if ref.Block.Role == assemblyline.SourceBlockTaskVerification {
		if executor.config.ValidateAcceptance == nil {
			return "", fmt.Errorf("adapter %s has no generated verification-body validator", executor.config.AdapterID)
		}
		validate = func(input assemblyline.FragmentGenerationInput, body string) (string, error) {
			declaration, err := executor.config.ValidateFragment(input, body)
			if err != nil {
				return "", err
			}
			if err := executor.config.ValidateAcceptance(ref, declaration); err != nil {
				return "", err
			}
			return declaration, nil
		}
	}
	return runDirectCodingLanguageFragmentWorker(
		runtime, modelName,
		directCodingLanguageGenerationJob{
			Subject: ref.Block.ID, Input: input,
			Validate: validate,
		},
	)
}
