package worker

import (
	"fmt"
	"github.com/gryph/omnidex/internal/assemblyline"
)

func finalizeInferenceFreePortableResult(
	job assemblyline.PortableJob,
	result assemblyline.PortableResult,
	execution exactStationExecution,
) (bool, error) {
	if !execution.InferenceFree {
		return false, nil
	}
	if execution.CallEvidenceID != 0 || execution.RootCallEvidenceID != 0 || execution.Model != "" || execution.Iteration != 0 ||
		execution.ProviderCalls != 0 ||
		execution.SourceState != "" || execution.Replayed ||
		execution.PersistedOutcome != "" || execution.PersistedValidationError != "" {
		return true, fmt.Errorf("portable work %s deterministic result carries provider authority", job.Kind)
	}
	if execution.WorkInput != string(job.Payload) || execution.WorkKind != job.Kind ||
		execution.Candidate != result.Candidate {
		return true, fmt.Errorf("portable work %s deterministic result differs from its code-owned receipt", job.Kind)
	}
	return true, result.ValidateFor(job)
}
