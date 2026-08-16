package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/gofragment"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

const repositoryGoVerificationRequiredChange = "Correct only the bound declaration so the exact staged Go verification diagnostic passes. Preserve all unrelated executable behavior."

func runRepositoryGoVerificationCorrection(
	runtime typedWorkerRuntime,
	modelName string,
	target repositoryfacts.ChangeTarget,
	current string,
	diagnostic string,
) (string, error) {
	if runtime.Context == nil || runtime.Execute == nil {
		return "", fmt.Errorf("repository Go verification correction requires a portable execution runtime")
	}
	if err := runtime.Context.Err(); err != nil {
		return "", fmt.Errorf("repository Go verification correction authority ended: %w", err)
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" || strings.TrimSpace(target.SymbolID) == "" {
		return "", fmt.Errorf("repository Go verification correction requires one model and exact target")
	}
	diagnostic, valid := validateRepositoryGoPathFreeDiagnostic(diagnostic)
	if !valid {
		return "", fmt.Errorf("repository Go verification correction requires one bounded path-free diagnostic")
	}
	capabilities := make([]string, len(target.DirectCapabilities))
	for index, capability := range target.DirectCapabilities {
		capabilities[index] = capability.Signature
	}
	permitted := target.PermittedCapabilitySymbols()
	input := assemblyline.FragmentCorrectionInput{
		Language: "go", Signature: target.Signature,
		Capabilities: capabilities, PermittedSymbols: permitted,
		CurrentDeclaration: strings.TrimSpace(current),
		RequiredChange:     repositoryGoVerificationRequiredChange,
		Diagnostic:         diagnostic,
	}
	job, err := assemblyline.NewFragmentCorrectionJob(input)
	if err != nil {
		return "", fmt.Errorf("build repository Go verification correction: %w", err)
	}
	prompt, _, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		return "", err
	}
	emitTypedWorker(runtime, typedWorkerEvent{
		State: typedWorkerStarted, Kind: typedWorkerFragment, Subject: target.SymbolID,
		Model: modelName, Attempt: 1, MaxAttempts: 1,
		PromptBytes: len([]byte(prompt)), CurrentBytes: len([]byte(input.CurrentDeclaration)),
		CorrectionBytes: len([]byte(diagnostic)),
		CapabilityBytes: len([]byte(strings.Join(capabilities, "\n"))),
	})
	result, err := runtime.Execute(job, modelName)
	if err != nil {
		return "", failRepositoryGoVerificationCorrection(runtime, modelName, target.SymbolID, err)
	}
	if err := result.ValidateFor(job); err != nil {
		err = finalizeTypedWorkerResult(runtime, job, result, err)
		return "", failRepositoryGoVerificationCorrection(runtime, modelName, target.SymbolID, err)
	}
	contract := gofragment.Contract{
		Signature: target.Signature, Current: input.CurrentDeclaration,
		PermittedSymbols: permitted,
	}
	currentCanonical, err := gofragment.ParseFunction(contract, input.CurrentDeclaration)
	if err != nil {
		err = finalizeTypedWorkerResult(runtime, job, result, err)
		return "", failRepositoryGoVerificationCorrection(runtime, modelName, target.SymbolID, err)
	}
	projected, err := gofragment.ProjectFunctionModelResponse(strings.TrimSpace(result.Candidate))
	if err != nil {
		err = finalizeTypedWorkerResult(runtime, job, result, err)
		return "", failRepositoryGoVerificationCorrection(runtime, modelName, target.SymbolID, err)
	}
	corrected, err := gofragment.ParseFunction(contract, projected)
	if err != nil {
		err = finalizeTypedWorkerResult(runtime, job, result, err)
		return "", failRepositoryGoVerificationCorrection(runtime, modelName, target.SymbolID, err)
	}
	if corrected == currentCanonical {
		err = finalizeTypedWorkerResult(runtime, job, result,
			fmt.Errorf("repository Go verification correction made no progress"))
		return "", failRepositoryGoVerificationCorrection(
			runtime, modelName, target.SymbolID,
			err,
		)
	}
	if err = finalizeTypedWorkerResult(runtime, job, result, nil); err != nil {
		return "", failRepositoryGoVerificationCorrection(runtime, modelName, target.SymbolID, err)
	}
	emitTypedWorker(runtime, typedWorkerEvent{
		State: typedWorkerCompleted, Kind: typedWorkerFragment, Subject: target.SymbolID,
		Model: modelName, Attempt: 1, MaxAttempts: 1,
	})
	return corrected, nil
}

func failRepositoryGoVerificationCorrection(
	runtime typedWorkerRuntime,
	modelName string,
	targetID string,
	err error,
) error {
	emitTypedWorker(runtime, typedWorkerEvent{
		State: typedWorkerFailed, Kind: typedWorkerFragment, Subject: targetID,
		Model: modelName, Attempt: 1, MaxAttempts: 1,
		Detail: trimForBudget(err.Error(), 1200),
	})
	return fmt.Errorf("repository Go verification correction failed: %w", err)
}
