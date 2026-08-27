package worker

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const (
	directCodingTypeScriptDeclarationReviewBytes = 5 * 1024
	maxTypeScriptOutputReviewBytes               = 128 * 1024
	directCodingTypeScriptModelAttempts          = 1
)

var errDirectCodingTypeScriptUnchangedCorrection = errors.New(
	"unchanged correction rejected; modify the current declaration to resolve the original failure",
)

func runDirectCodingTypeScriptFragmentWorker(
	runtime typedWorkerRuntime,
	modelName string,
	job directCodingTypeScriptFragmentJob,
) (string, error) {
	if runtime.Context == nil || runtime.Execute == nil {
		return "", fmt.Errorf("TypeScript fragment worker requires a portable execution runtime")
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return "", fmt.Errorf("TypeScript fragment worker requires one configured model")
	}
	baseJob, err := newDirectCodingTypeScriptPortableJob(job)
	if err != nil {
		return "", failDirectCodingTypeScriptFragmentWorker(runtime, modelName, job.block.ID, 0, err)
	}
	guided := strings.TrimSpace(job.repairGuidance) != ""
	proseContext := []string{job.block.Contract}
	sourceContext := []string{job.block.Signature, job.available}
	if guided {
		proseContext = []string{job.repairGuidance}
		sourceContext = []string{job.current}
		if job.repairRegion != nil {
			sourceContext = []string{job.repairRegion.Source}
		}
	} else {
		proseContext = append([]string{job.dialect}, proseContext...)
		sourceContext = append(sourceContext, job.block.Globals...)
	}
	if err := assemblyline.ValidatePathFreeModelContextWithProvenance(
		"TypeScript fragment", runtime.PathProvenance, proseContext...,
	); err != nil {
		return "", failDirectCodingTypeScriptFragmentWorker(runtime, modelName, job.block.ID, 0, err)
	}
	if err := assemblyline.ValidatePathFreeSourceModelContextWithProvenance(
		"TypeScript fragment", runtime.PathProvenance, sourceContext...,
	); err != nil {
		return "", failDirectCodingTypeScriptFragmentWorker(runtime, modelName, job.block.ID, 0, err)
	}
	if err := runtime.Context.Err(); err != nil {
		return "", failDirectCodingTypeScriptFragmentWorker(runtime, modelName, job.block.ID, 0, err)
	}
	prompt, _, err := assemblyline.RenderPortableJob(baseJob)
	if err != nil {
		return "", failDirectCodingTypeScriptFragmentWorker(runtime, modelName, job.block.ID, 0, err)
	}
	currentBytes := len(strings.TrimSpace(job.current))
	if job.repairRegion != nil {
		currentBytes = len(job.repairRegion.Source)
	}
	correctionBytes := 0
	if guided {
		correctionBytes = len(strings.TrimSpace(job.repairGuidance))
	}
	emitTypedWorker(runtime, typedWorkerEvent{
		State: typedWorkerStarted, Kind: typedWorkerFragment, Subject: job.block.ID,
		Model: modelName, Attempt: 1, MaxAttempts: directCodingTypeScriptModelAttempts,
		PromptBytes: len(prompt), CapabilityBytes: len(strings.TrimSpace(job.available)),
		CurrentBytes: currentBytes, CorrectionBytes: correctionBytes,
		Warning: directCodingTypeScriptDeclarationSizeWarning(currentBytes),
	})
	result, err := runtime.Execute(baseJob, modelName)
	if err != nil {
		return "", failDirectCodingTypeScriptFragmentWorker(runtime, modelName, job.block.ID, 1, err)
	}
	if err = result.ValidateFor(baseJob); err != nil {
		err = finalizeTypedWorkerResult(runtime, baseJob, result, err)
		return "", failDirectCodingTypeScriptFragmentWorker(runtime, modelName, job.block.ID, 1, err)
	}
	candidate := ""
	initialProjectionAccepted := false
	if job.repairRegion != nil {
		var replacement string
		replacement, err = assemblyline.ProjectTypeScriptFragmentRepairResponse(
			*job.repairRegion, result.Candidate,
		)
		if err == nil {
			candidate, err = assemblyline.ApplyTypeScriptFragmentRepairRegion(
				strings.TrimSpace(job.current), *job.repairRegion, replacement,
			)
		}
		if err == nil {
			var projection assemblyline.PortableResultProjection
			projection, err = assemblyline.NewExactPortableResultProjection(result.Candidate)
			if err == nil {
				result.Projection = &projection
			}
		}
	} else {
		var projection assemblyline.TypeScriptFunctionProjection
		projection, err = assemblyline.ProjectTypeScriptFunctionModelResponse(
			assemblyline.TypeScriptFunctionContract{
				Signature: job.block.Signature, TSX: job.tsx, Policy: job.block.Policy,
			},
			result.Candidate,
		)
		candidate = projection.Source
		if err == nil {
			var portableProjection assemblyline.PortableResultProjection
			portableProjection, err = projection.PortableResultProjection()
			if err == nil {
				result.Projection = &portableProjection
				initialProjectionAccepted = !guided
			}
		}
	}
	candidate = strings.TrimSpace(candidate)
	candidatePathFree := false
	if candidate != "" {
		pathErr := assemblyline.ValidatePathFreeSourceModelContextWithProvenance(
			"TypeScript fragment candidate", runtime.PathProvenance, candidate,
		)
		if pathErr == nil {
			candidatePathFree = true
		} else if err == nil {
			err = pathErr
		} else {
			err = errors.Join(err, pathErr)
		}
	}
	if err == nil && guided && candidate == strings.TrimSpace(job.current) {
		err = errDirectCodingTypeScriptUnchangedCorrection
	}
	if err == nil {
		_, err = assemblyline.ParseTypeScriptFunction(assemblyline.TypeScriptFunctionContract{
			Signature: job.block.Signature, TSX: job.tsx, Policy: job.block.Policy,
		}, candidate)
	}
	if err != nil {
		validationErr := err
		if !guided && initialProjectionAccepted && candidatePathFree {
			validationErr = &directCodingTypeScriptInitialFragmentRejection{
				Candidate: candidate,
				Failure:   err,
			}
		}
		rejectionErr := finalizeTypedWorkerResult(runtime, baseJob, result, validationErr)
		emitTypedWorker(runtime, typedWorkerEvent{
			State: typedWorkerRejected, Kind: typedWorkerFragment, Subject: job.block.ID,
			Model: modelName, Attempt: 1, MaxAttempts: directCodingTypeScriptModelAttempts,
			Warning: joinTypedWorkerWarnings(
				directCodingTypeScriptDeclarationSizeWarning(len(candidate)),
				directCodingTypeScriptProjectionWarning(result.Projection),
			),
			Detail: trimForBudget(rejectionErr.Error(), 1200),
		})
		return "", failDirectCodingTypeScriptFragmentWorker(
			runtime, modelName, job.block.ID, 1, rejectionErr,
		)
	}
	if err = finalizeTypedWorkerResult(runtime, baseJob, result, nil); err != nil {
		return "", failDirectCodingTypeScriptFragmentWorker(runtime, modelName, job.block.ID, 1, err)
	}
	emitTypedWorker(runtime, typedWorkerEvent{
		State: typedWorkerCompleted, Kind: typedWorkerFragment, Subject: job.block.ID,
		Model: modelName, Attempt: 1, MaxAttempts: directCodingTypeScriptModelAttempts,
		Warning: joinTypedWorkerWarnings(
			directCodingTypeScriptDeclarationSizeWarning(len(candidate)),
			directCodingTypeScriptProjectionWarning(result.Projection),
		),
	})
	return candidate, nil
}

func newDirectCodingTypeScriptPortableJob(job directCodingTypeScriptFragmentJob) (assemblyline.PortableJob, error) {
	guidance := strings.TrimSpace(job.repairGuidance)
	if strings.TrimSpace(job.current) == "" && job.repairRegion == nil {
		if guidance != "" || strings.TrimSpace(job.failure) != "" ||
			strings.TrimSpace(job.requiredChange) != "" {
			return assemblyline.PortableJob{}, fmt.Errorf(
				"TypeScript fragment generation cannot carry repair authority",
			)
		}
		capabilities := make([]string, 0, 1)
		if available := strings.TrimSpace(job.available); available != "" {
			capabilities = append(capabilities, available)
		}
		return assemblyline.NewFragmentGenerationJob(assemblyline.FragmentGenerationInput{
			Language: "typescript", Dialect: strings.TrimSpace(job.dialect),
			Signature: strings.TrimSpace(job.block.Signature),
			Behavior:  strings.TrimSpace(job.block.Contract), Capabilities: capabilities,
			PermittedSymbols: append([]string(nil), job.block.Globals...),
		})
	}
	if guidance == "" {
		return assemblyline.PortableJob{}, fmt.Errorf(
			"unguided TypeScript fragment correction is forbidden; derive one repair instruction first",
		)
	}
	if strings.TrimSpace(job.failure) != "" || strings.TrimSpace(job.requiredChange) != "" {
		return assemblyline.PortableJob{}, fmt.Errorf(
			"guided TypeScript fragment correction cannot carry a raw diagnostic or required change",
		)
	}
	portableCurrent := strings.TrimSpace(job.current)
	if job.repairRegion != nil {
		portableCurrent = ""
	}
	return assemblyline.NewFragmentCorrectionJob(assemblyline.FragmentCorrectionInput{
		Language: "typescript", Signature: strings.TrimSpace(job.block.Signature),
		CurrentDeclaration: portableCurrent,
		RepairRegion:       job.repairRegion,
		RepairGuidance:     guidance,
	})
}

func failDirectCodingTypeScriptFragmentWorker(
	runtime typedWorkerRuntime,
	modelName string,
	blockID string,
	attempt int,
	err error,
) error {
	emitTypedWorker(runtime, typedWorkerEvent{
		State: typedWorkerFailed, Kind: typedWorkerFragment, Subject: blockID,
		Model: modelName, Attempt: attempt, MaxAttempts: directCodingTypeScriptModelAttempts,
		Detail: trimForBudget(err.Error(), 1200),
	})
	return fmt.Errorf("TypeScript fragment worker failed: %w", err)
}
