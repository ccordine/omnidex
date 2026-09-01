package worker

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func runDirectCodingTypeScriptFragmentWorker(
	runtime typedWorkerRuntime,
	modelName string,
	job directCodingTypeScriptFragmentJob,
) (string, error) {
	if modelName == "" {
		return "", fmt.Errorf("TypeScript fragment worker requires one configured model")
	}
	input, err := directCodingTypeScriptFragmentInput(job)
	if err != nil {
		return "", err
	}
	return runDirectCodingLanguageFragmentWorker(
		runtime,
		modelName,
		directCodingLanguageGenerationJob{
			Subject: job.block.ID,
			Input:   input,
			Normalize: func(
				_ assemblyline.FragmentGenerationInput,
				response string,
			) (string, error) {
				return assemblyline.ExtractTypeScriptFunctionBodyResponse(
					assemblyline.TypeScriptFunctionContract{
						Signature: job.block.Signature,
						TSX:       job.tsx,
					},
					response,
				)
			},
			Validate: func(
				input assemblyline.FragmentGenerationInput,
				body string,
			) (string, error) {
				fragment, err := assemblyline.ParseTypeScriptFunctionBody(
					assemblyline.TypeScriptFunctionContract{
						Signature: job.block.Signature,
						TSX:       job.tsx,
						Policy:    job.block.Policy,
					},
					body,
				)
				if err != nil {
					var defect *assemblyline.SourceBodyDefect
					if errors.As(err, &defect) && defect.RequiresIdentifierReplacement() {
						failedStart, failedEnd, mutableErr := defect.MutableRange(body)
						if mutableErr != nil {
							return "", mutableErr
						}
						failed := body[failedStart:failedEnd]
						replacements, replacementErr := directCodingTypeScriptIdentifierChoices(
							input, body, failed, failedStart, failedEnd, job.tsx,
							job.block.Policy,
						)
						if replacementErr != nil {
							return "", fmt.Errorf(
								"enumerate exact TypeScript identifier replacements: %w",
								replacementErr,
							)
						}
						bound, bindErr := defect.WithIdentifierReplacements(replacements)
						if bindErr != nil {
							return "", bindErr
						}
						return "", bound
					}
					return "", err
				}
				candidate := strings.TrimSpace(fragment.Source)
				if job.validateInitialCandidate != nil {
					if err := job.validateInitialCandidate(candidate); err != nil {
						return "", fmt.Errorf(
							"TypeScript implementation candidate has no exact public interaction surface: %w",
							err,
						)
					}
				}
				return candidate, nil
			},
		},
	)
}

func generateDirectCodingTypeScriptBlockWithRuntime(
	runtime typedWorkerRuntime,
	fragmentModel string,
	job directCodingTypeScriptFragmentJob,
) (string, error) {
	return runDirectCodingTypeScriptFragmentWorker(runtime, fragmentModel, job)
}
