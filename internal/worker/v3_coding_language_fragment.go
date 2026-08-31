package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingLanguageFragmentValidator func(
	assemblyline.FragmentGenerationInput,
	string,
) (string, error)

type directCodingLanguageFragmentProjector func(
	string,
) (assemblyline.PortableResultProjection, error)

type directCodingLanguageGenerationJob struct {
	Subject  string
	Input    assemblyline.FragmentGenerationInput
	Project  directCodingLanguageFragmentProjector
	Validate directCodingLanguageFragmentValidator
}

// runDirectCodingLanguageFragmentWorker resolves one path-blind source
// declaration and grants the model no correction, file, tool, or workflow
// authority. The selected language parser owns acceptance.
func runDirectCodingLanguageFragmentWorker(
	runtime typedWorkerRuntime,
	modelName string,
	job directCodingLanguageGenerationJob,
) (string, error) {
	if runtime.Context == nil || runtime.Execute == nil {
		return "", fmt.Errorf("language fragment worker requires a portable execution runtime")
	}
	if runtime.MaxAttempts != 1 {
		return "", fmt.Errorf("language fragment worker requires exactly one generation attempt")
	}
	if modelName == "" || strings.TrimSpace(job.Subject) == "" ||
		job.Project == nil || job.Validate == nil {
		return "", fmt.Errorf(
			"language fragment worker requires one model, opaque subject, projector, and parser",
		)
	}
	portable, err := assemblyline.NewFragmentGenerationJob(job.Input)
	if err != nil {
		return "", failDirectCodingLanguageGeneration(runtime, modelName, job, err)
	}
	if err := assemblyline.ValidatePathFreeModelContextWithProvenance(
		"language fragment behavior", runtime.PathProvenance, job.Input.Dialect, job.Input.Behavior,
	); err != nil {
		return "", failDirectCodingLanguageGeneration(runtime, modelName, job, err)
	}
	sourceContext := []string{job.Input.Signature}
	sourceContext = append(sourceContext, job.Input.Capabilities...)
	sourceContext = append(sourceContext, job.Input.PermittedSymbols...)
	if err := assemblyline.ValidatePathFreeSourceModelContextWithProvenance(
		"language fragment", runtime.PathProvenance, sourceContext...,
	); err != nil {
		return "", failDirectCodingLanguageGeneration(runtime, modelName, job, err)
	}
	prompt, err := assemblyline.RenderPortableJob(portable)
	if err != nil {
		return "", failDirectCodingLanguageGeneration(runtime, modelName, job, err)
	}
	emitTypedWorker(runtime, typedWorkerEvent{
		State: typedWorkerStarted, Kind: typedWorkerFragment, Subject: job.Subject,
		Model: modelName, Attempt: 1, MaxAttempts: 1, PromptBytes: len(prompt),
		CapabilityBytes: languageGenerationCapabilityBytes(job.Input),
	})
	result, err := runtime.Execute(portable, modelName)
	if err != nil {
		return "", failDirectCodingLanguageGeneration(runtime, modelName, job, err)
	}
	if err := result.ValidateFor(portable); err != nil {
		err = finalizeTypedWorkerResult(runtime, portable, result, err)
		return "", failDirectCodingLanguageGeneration(runtime, modelName, job, err)
	}
	rawCandidate := result.Candidate
	if err := validateDirectCodingLanguageFragmentCandidatePathBoundary(
		job.Input.Language, runtime.PathProvenance, rawCandidate,
	); err != nil {
		err = finalizeTypedWorkerResult(runtime, portable, result, err)
		return "", failDirectCodingLanguageGeneration(runtime, modelName, job, err)
	}
	projection, err := job.Project(rawCandidate)
	if err != nil {
		err = finalizeTypedWorkerResult(runtime, portable, result, err)
		return "", failDirectCodingLanguageGeneration(runtime, modelName, job, err)
	}
	result.Projection = &projection
	candidate := projection.Source
	_, err = job.Validate(job.Input, candidate)
	if err != nil {
		rejection := &directCodingLanguageFragmentRejection{
			Candidate: candidate, Failure: err,
		}
		err = finalizeTypedWorkerResult(runtime, portable, result, rejection)
		return "", failDirectCodingLanguageGeneration(runtime, modelName, job, err)
	}
	if err := finalizeTypedWorkerResult(runtime, portable, result, nil); err != nil {
		return "", failDirectCodingLanguageGeneration(runtime, modelName, job, err)
	}
	emitTypedWorker(runtime, typedWorkerEvent{
		State: typedWorkerCompleted, Kind: typedWorkerFragment, Subject: job.Subject,
		Model: modelName, Attempt: 1, MaxAttempts: 1,
	})
	return candidate, nil
}

func validateDirectCodingLanguageFragmentCandidatePathBoundary(
	language string,
	provenance assemblyline.ArtifactIdentityProvenance,
	candidate string,
) error {
	if language == assemblyline.TextFragmentLanguage {
		return assemblyline.ValidatePathFreeModelContextWithProvenance(
			"language fragment candidate", provenance, candidate,
		)
	}
	return assemblyline.ValidatePathFreeSourceModelContextWithProvenance(
		"language fragment candidate", provenance, candidate,
	)
}

func languageGenerationCapabilityBytes(input assemblyline.FragmentGenerationInput) int {
	return len(strings.Join(input.Capabilities, "\n")) +
		len(strings.Join(input.PermittedSymbols, "\n"))
}

func failDirectCodingLanguageGeneration(
	runtime typedWorkerRuntime,
	modelName string,
	job directCodingLanguageGenerationJob,
	err error,
) error {
	emitTypedWorker(runtime, typedWorkerEvent{
		State: typedWorkerFailed, Kind: typedWorkerFragment, Subject: job.Subject,
		Model: modelName, Attempt: 1, MaxAttempts: 1,
		Detail: trimForBudget(err.Error(), 1200),
	})
	return fmt.Errorf("%s fragment generation failed: %w", job.Input.Language, err)
}

func directCodingLanguageFragmentInput(
	stage *directCodingProgram,
	ref assemblyline.SourceBlockRef,
	language string,
) (assemblyline.FragmentGenerationInput, error) {
	if stage == nil || !ref.Block.Generated() {
		return assemblyline.FragmentGenerationInput{}, fmt.Errorf(
			"%s generation requires one generated source block", language,
		)
	}
	blocks := make(map[string]assemblyline.SourceBlock)
	found := false
	for _, document := range stage.Source.Documents {
		for _, block := range document.Blocks {
			blocks[block.ID] = block
			if block.ID == ref.Block.ID && document.ID == ref.Document.ID {
				found = true
			}
		}
	}
	if !found {
		return assemblyline.FragmentGenerationInput{}, fmt.Errorf(
			"%s block %s is absent from isolated stage", language, ref.Block.ID,
		)
	}
	capabilities := make([]string, 0, len(ref.Block.Capabilities))
	for _, capabilityID := range ref.Block.Capabilities {
		capability, exists := blocks[capabilityID]
		if !exists {
			return assemblyline.FragmentGenerationInput{}, fmt.Errorf(
				"%s block %s lacks capability %s", language, ref.Block.ID, capabilityID,
			)
		}
		if capability.Generated() && strings.TrimSpace(stage.Generated[capabilityID]) == "" {
			return assemblyline.FragmentGenerationInput{}, fmt.Errorf(
				"%s capability %s has no accepted declaration", language, capabilityID,
			)
		}
		capabilities = append(capabilities, capability.API)
	}
	return assemblyline.FragmentGenerationInput{
		Language: language, Dialect: stage.Project.Dialect, Signature: ref.Block.Signature,
		Behavior: ref.Block.Contract, Capabilities: capabilities,
		PermittedSymbols: append([]string(nil), ref.Block.Globals...),
	}, nil
}
