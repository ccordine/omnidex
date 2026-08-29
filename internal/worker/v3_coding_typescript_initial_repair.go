package worker

import (
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

// directCodingTypeScriptInitialFragmentRejection retains only an exact,
// cleanly projected declaration and its parser failure. Projection failures
// can describe authority outside that declaration, so they remain terminal.
// The rejection grants no workflow or mutation authority and is never created
// for a guided response.
type directCodingTypeScriptInitialFragmentRejection struct {
	Candidate string
	Failure   error
}

func (rejection *directCodingTypeScriptInitialFragmentRejection) Error() string {
	if rejection == nil || rejection.Failure == nil {
		return "TypeScript initial fragment rejection is incomplete"
	}
	return rejection.Failure.Error()
}

func (rejection *directCodingTypeScriptInitialFragmentRejection) Unwrap() error {
	if rejection == nil {
		return nil
	}
	return rejection.Failure
}

func generateDirectCodingTypeScriptBlockWithRuntime(
	runtime typedWorkerRuntime,
	fragmentModel string,
	repairModels directCodingTypeScriptRepairModelResolver,
	events directCodingTypeScriptRepairEvents,
	job directCodingTypeScriptFragmentJob,
) (string, error) {
	source, err := runDirectCodingTypeScriptFragmentWorker(runtime, fragmentModel, job)
	if err == nil {
		return source, nil
	}
	var rejection *directCodingTypeScriptInitialFragmentRejection
	if job.block.Role == assemblyline.SourceBlockTaskVerification ||
		!errors.As(err, &rejection) {
		return "", err
	}
	diagnostic, diagnosticErr := directCodingLanguageParserRepairDiagnostic(
		runtime.PathProvenance, rejection.Failure,
	)
	if diagnosticErr != nil {
		return "", errors.Join(err, diagnosticErr)
	}
	if repairModels == nil {
		return "", fmt.Errorf("initial TypeScript fragment repair model routing is unavailable")
	}
	guidanceModel, correctionModel, modelErr := repairModels()
	if modelErr != nil {
		return "", modelErr
	}
	repairRuntime := runtime
	repairRuntime.MaxAttempts = 1
	return convergeDirectCodingTypeScriptGuidedRepairWithRuntime(
		repairRuntime, guidanceModel, correctionModel, events,
		job.block, job.tsx, job.dialect, job.available, rejection.Candidate, nil, diagnostic,
	)
}
