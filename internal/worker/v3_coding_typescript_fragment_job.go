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
	hasInitialValidator := job.validateInitialCandidate != nil
	if hasInitialValidator && job.block.Role != assemblyline.SourceBlockTaskImplementation &&
		job.block.Role != assemblyline.SourceBlockTaskVerification {
		return assemblyline.PortableJob{}, fmt.Errorf(
			"TypeScript source-only candidate validation is restricted to implementation and verification declarations",
		)
	}
	if strings.TrimSpace(job.current) == "" {
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
	return assemblyline.NewFragmentCorrectionJob(assemblyline.FragmentCorrectionInput{
		Language: "typescript", Signature: strings.TrimSpace(job.block.Signature),
		CurrentDeclaration: strings.TrimSpace(job.current),
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
