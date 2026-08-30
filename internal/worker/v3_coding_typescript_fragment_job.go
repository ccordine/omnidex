package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func newDirectCodingTypeScriptPortableJob(
	job directCodingTypeScriptFragmentJob,
) (assemblyline.PortableJob, error) {
	guidance := strings.TrimSpace(job.repairGuidance)
	hasPublicSurface := job.publicInteractionSurface != nil
	hasInitialValidator := job.validateInitialCandidate != nil
	if hasPublicSurface && !hasInitialValidator {
		return assemblyline.PortableJob{}, fmt.Errorf(
			"TypeScript public interaction surface requires a candidate validator",
		)
	}
	if hasPublicSurface && job.block.Role != assemblyline.SourceBlockTaskVerification {
		return assemblyline.PortableJob{}, fmt.Errorf(
			"TypeScript public interaction surface is restricted to verification declarations",
		)
	}
	if hasInitialValidator && !hasPublicSurface &&
		job.block.Role != assemblyline.SourceBlockTaskImplementation {
		return assemblyline.PortableJob{}, fmt.Errorf(
			"TypeScript source-only candidate validation is restricted to implementation declarations",
		)
	}
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
			PermittedSymbols:         append([]string(nil), job.block.Globals...),
			PublicInteractionSurface: job.publicInteractionSurface,
		})
	}
	if hasPublicSurface {
		return assemblyline.PortableJob{}, fmt.Errorf(
			"TypeScript correction cannot receive a public interaction surface",
		)
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
