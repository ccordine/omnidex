package worker

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/gofragment"
)

type directCodingLanguageFragmentValidator func(
	assemblyline.FragmentGenerationInput,
	string,
) (string, error)

type directCodingLanguageResponseNormalizer func(
	assemblyline.FragmentGenerationInput,
	string,
) (string, error)

type directCodingLanguageGenerationJob struct {
	Subject   string
	Input     assemblyline.FragmentGenerationInput
	Normalize directCodingLanguageResponseNormalizer
	Validate  directCodingLanguageFragmentValidator
}

// runDirectCodingLanguageFragmentWorker resolves one path-blind implementation
// body. Code owns the declaration and continues only this persisted job when
// deterministic validation identifies one exact defect.
func runDirectCodingLanguageFragmentWorker(
	runtime typedWorkerRuntime,
	modelName string,
	job directCodingLanguageGenerationJob,
) (string, error) {
	if runtime.Context == nil || runtime.Execute == nil || runtime.Correct == nil ||
		runtime.Release == nil || runtime.Finalize == nil {
		return "", fmt.Errorf("language fragment worker requires a portable execution runtime")
	}
	if runtime.MaxAttempts != assemblyline.MaxSourceBodyAttempts {
		return "", fmt.Errorf(
			"language fragment worker requires exactly %d bounded body attempts",
			assemblyline.MaxSourceBodyAttempts,
		)
	}
	if modelName == "" || strings.TrimSpace(job.Subject) == "" || job.Validate == nil {
		return "", fmt.Errorf(
			"language fragment worker requires one model, opaque subject, and parser",
		)
	}
	portable, err := assemblyline.NewFragmentGenerationJob(job.Input)
	if err != nil {
		return "", failDirectCodingLanguageGeneration(runtime, modelName, job, 0, err)
	}
	if err := assemblyline.ValidatePathFreeModelContextWithProvenance(
		"language fragment behavior", runtime.PathProvenance, job.Input.Dialect, job.Input.Behavior,
	); err != nil {
		return "", failDirectCodingLanguageGeneration(runtime, modelName, job, 0, err)
	}
	sourceContext := []string{job.Input.Signature}
	sourceContext = append(sourceContext, job.Input.Capabilities...)
	sourceContext = append(sourceContext, job.Input.PermittedSymbols...)
	if err := assemblyline.ValidatePathFreeSourceModelContextWithProvenance(
		"language fragment", runtime.PathProvenance, sourceContext...,
	); err != nil {
		return "", failDirectCodingLanguageGeneration(runtime, modelName, job, 0, err)
	}
	prompt, err := assemblyline.RenderPortableJob(portable)
	if err != nil {
		return "", failDirectCodingLanguageGeneration(runtime, modelName, job, 0, err)
	}
	var correction assemblyline.SourceBodyCorrection
	for attempt := 1; attempt <= runtime.MaxAttempts; attempt++ {
		correctionBytes := 0
		if attempt > 1 {
			correctionInput, correctionErr := correction.ModelInput()
			if correctionErr != nil {
				return "", failDirectCodingLanguageGeneration(
					runtime, modelName, job, attempt,
					releaseDirectCodingSourceBodyContext(runtime, portable, correctionErr),
				)
			}
			correctionBytes = len(correctionInput)
		}
		emitTypedWorker(runtime, typedWorkerEvent{
			State: typedWorkerStarted, Kind: typedWorkerFragment, Subject: job.Subject,
			Model: modelName, Attempt: attempt, MaxAttempts: runtime.MaxAttempts,
			PromptBytes:     len(prompt),
			CapabilityBytes: languageGenerationCapabilityBytes(job.Input),
			CorrectionBytes: correctionBytes,
		})
		var result assemblyline.PortableResult
		if attempt == 1 {
			result, err = runtime.Execute(portable, modelName)
		} else {
			result, err = runtime.Correct(portable, modelName, correction)
		}
		if err != nil {
			return "", failDirectCodingLanguageGeneration(
				runtime, modelName, job, attempt,
				releaseDirectCodingSourceBodyContext(runtime, portable, err),
			)
		}
		validationErr := result.ValidateFor(portable)
		body := ""
		providerBody := ""
		if validationErr == nil && attempt == 1 {
			body, validationErr = normalizeDirectCodingLanguageResponse(
				job, result.Candidate,
			)
			providerBody = body
		} else if validationErr == nil {
			body, validationErr = correction.Apply(result.Candidate)
			providerBody = body
		}
		candidate := ""
		var nextCorrection *assemblyline.SourceBodyCorrection
		if validationErr == nil {
			candidate, body, nextCorrection, validationErr =
				validateDirectCodingLanguageBody(
					runtime.PathProvenance, job, body,
				)
		}
		if validationErr == nil && nextCorrection != nil {
			validationErr = fmt.Errorf(
				"language fragment validator returned a correction without a defect",
			)
		}
		if validationErr == nil {
			if err := finalizeTypedWorkerResult(runtime, portable, result, nil); err != nil {
				return "", failDirectCodingLanguageGeneration(
					runtime, modelName, job, attempt, err,
				)
			}
			emitTypedWorker(runtime, typedWorkerEvent{
				State: typedWorkerCompleted, Kind: typedWorkerFragment, Subject: job.Subject,
				Model: modelName, Attempt: attempt, MaxAttempts: runtime.MaxAttempts,
			})
			return candidate, nil
		}
		if err := runtime.Finalize(portable, result, validationErr); err != nil {
			return "", failDirectCodingLanguageGeneration(
				runtime, modelName, job, attempt, err,
			)
		}
		if body != providerBody && nextCorrection != nil {
			if runtime.AdvanceSource == nil {
				return "", failDirectCodingLanguageGeneration(
					runtime, modelName, job, attempt,
					releaseDirectCodingSourceBodyContext(
						runtime,
						portable,
						fmt.Errorf(
							"persist a deterministic source-span advance before later correction",
						),
					),
				)
			}
			if err := runtime.AdvanceSource(
				portable, modelName, providerBody, body,
			); err != nil {
				return "", failDirectCodingLanguageGeneration(
					runtime, modelName, job, attempt,
					releaseDirectCodingSourceBodyContext(runtime, portable, err),
				)
			}
		}
		emitTypedWorker(runtime, typedWorkerEvent{
			State: typedWorkerRejected, Kind: typedWorkerFragment, Subject: job.Subject,
			Model: modelName, Attempt: attempt, MaxAttempts: runtime.MaxAttempts,
			Detail: trimForBudget(validationErr.Error(), 1200),
		})
		if attempt == runtime.MaxAttempts {
			return "", failDirectCodingLanguageGeneration(
				runtime, modelName, job, attempt,
				releaseDirectCodingSourceBodyContext(runtime, portable, validationErr),
			)
		}
		if nextCorrection == nil {
			err = fmt.Errorf(
				"validation defect has no code-proven mutable source span: %w",
				validationErr,
			)
			return "", failDirectCodingLanguageGeneration(
				runtime, modelName, job, attempt,
				releaseDirectCodingSourceBodyContext(runtime, portable, err),
			)
		}
		correction = *nextCorrection
		modelInput, err := correction.ModelInput()
		if err != nil {
			return "", failDirectCodingLanguageGeneration(
				runtime, modelName, job, attempt,
				releaseDirectCodingSourceBodyContext(runtime, portable, err),
			)
		}
		if err := assemblyline.ValidatePathFreeSourceModelContextWithProvenance(
			"source-span correction", runtime.PathProvenance, modelInput,
		); err != nil {
			return "", failDirectCodingLanguageGeneration(
				runtime, modelName, job, attempt,
				releaseDirectCodingSourceBodyContext(runtime, portable, err),
			)
		}
	}
	return "", fmt.Errorf("language fragment worker exhausted an unreachable attempt state")
}

func validateDirectCodingLanguageBody(
	provenance assemblyline.ArtifactIdentityProvenance,
	job directCodingLanguageGenerationJob,
	body string,
) (
	candidate string,
	validatedBody string,
	correction *assemblyline.SourceBodyCorrection,
	validationErr error,
) {
	seen := map[string]struct{}{body: {}}
	for {
		candidate, validationErr = job.Validate(job.Input, body)
		if validationErr == nil {
			validationErr = validateDirectCodingLanguageFragmentCandidatePathBoundary(
				job.Input.Language, provenance, body,
			)
		}
		if validationErr == nil {
			return candidate, body, nil, nil
		}
		var spanDefect *assemblyline.SourceBodyDefect
		if !errors.As(validationErr, &spanDefect) {
			return "", body, nil, validationErr
		}
		next, err := spanDefect.Correction(body)
		if err != nil {
			return "", body, nil, fmt.Errorf(
				"bind exact source-span correction: %w", err,
			)
		}
		updated, resolved, err := next.ApplySoleReplacement()
		if err != nil {
			return "", body, nil, fmt.Errorf(
				"apply sole code-owned identifier replacement: %w", err,
			)
		}
		if !resolved {
			return "", body, &next, validationErr
		}
		if _, duplicate := seen[updated]; duplicate {
			return "", body, nil, fmt.Errorf(
				"deterministic source-span replacement entered a source-state cycle",
			)
		}
		seen[updated] = struct{}{}
		body = updated
	}
}

func normalizeDirectCodingLanguageResponse(
	job directCodingLanguageGenerationJob,
	response string,
) (string, error) {
	if job.Normalize != nil {
		return job.Normalize(job.Input, response)
	}
	if job.Input.Language == assemblyline.TextFragmentLanguage {
		return assemblyline.NormalizeTextFragmentResponse(response)
	}
	switch job.Input.Language {
	case "go":
		return gofragment.ExtractNewFunctionBodyResponse(job.Input.Signature, response)
	case "typescript":
		return assemblyline.ExtractTypeScriptFunctionBodyResponse(
			assemblyline.TypeScriptFunctionContract{Signature: job.Input.Signature}, response,
		)
	case "javascript":
		return assemblyline.ExtractJavaScriptSourceBodyResponse(job.Input.Signature, response)
	case "java":
		return assemblyline.ExtractJavaSourceBodyResponse(job.Input.Signature, response)
	case "rust":
		return assemblyline.ExtractRustSourceBodyResponse(job.Input.Signature, response)
	default:
		return "", fmt.Errorf(
			"language %q has no ordinary source-body response extractor", job.Input.Language,
		)
	}
}

func validateDirectCodingLanguageFragmentCandidatePathBoundary(
	language string,
	provenance assemblyline.ArtifactIdentityProvenance,
	candidate string,
) error {
	var err error
	if language == assemblyline.TextFragmentLanguage {
		err = assemblyline.ValidatePathFreeModelContextWithProvenance(
			"language fragment candidate", provenance, candidate,
		)
	} else {
		err = assemblyline.ValidatePathFreeSourceModelContextWithProvenance(
			"language fragment candidate", provenance, candidate,
		)
	}
	if err != nil {
		return fmt.Errorf(
			"implementation body contains a filesystem identity; filesystem identities are code-owned",
		)
	}
	return nil
}

func languageGenerationCapabilityBytes(input assemblyline.FragmentGenerationInput) int {
	return len(strings.Join(input.Capabilities, "\n")) +
		len(strings.Join(input.PermittedSymbols, "\n"))
}

func failDirectCodingLanguageGeneration(
	runtime typedWorkerRuntime,
	modelName string,
	job directCodingLanguageGenerationJob,
	attempt int,
	err error,
) error {
	emitTypedWorker(runtime, typedWorkerEvent{
		State: typedWorkerFailed, Kind: typedWorkerFragment, Subject: job.Subject,
		Model: modelName, Attempt: attempt, MaxAttempts: runtime.MaxAttempts,
		Detail: trimForBudget(err.Error(), 1200),
	})
	return fmt.Errorf("%s fragment generation failed: %w", job.Input.Language, err)
}

func releaseDirectCodingSourceBodyContext(
	runtime typedWorkerRuntime,
	job assemblyline.PortableJob,
	failure error,
) error {
	if runtime.Release == nil {
		return failure
	}
	if err := runtime.Release(job); err != nil {
		return fmt.Errorf("%v; release persisted source-body context: %w", failure, err)
	}
	return failure
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
