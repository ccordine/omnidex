package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
)

// exactStationLineageRoot follows bounded persisted parent IDs. Each child
// retains the same job, station, and model route; only its local span changes.
func (s *Service) exactStationLineageRoot(
	ctx context.Context,
	call queue.LLMCallEvidence,
) (queue.LLMCallEvidence, error) {
	if ctx == nil || s == nil || s.repo == nil {
		return queue.LLMCallEvidence{}, fmt.Errorf("station lineage requires context and PostgreSQL")
	}
	if call.Iteration < 1 || call.Iteration > assemblyline.MaxSourceBodyAttempts {
		return queue.LLMCallEvidence{}, fmt.Errorf("station correction lineage exceeds its attempt bound")
	}
	for call.Iteration > 1 {
		if call.ParentCallEvidenceID < 1 || call.WorkInput != nil {
			return queue.LLMCallEvidence{}, fmt.Errorf("station correction has invalid parent or duplicate initial input")
		}
		parent, err := s.repo.GetLLMCallEvidence(ctx, call.ParentCallEvidenceID)
		if err != nil {
			return queue.LLMCallEvidence{}, fmt.Errorf("read station correction parent: %w", err)
		}
		if parent.Iteration != call.Iteration-1 || parent.JobID != call.JobID ||
			parent.Generation != call.Generation || parent.StepID != call.StepID ||
			parent.WorkKind != call.WorkKind || parent.Scope != call.Scope ||
			parent.RequestedModel != call.RequestedModel || parent.Model != call.Model ||
			parent.Protocol != call.Protocol {
			return queue.LLMCallEvidence{}, fmt.Errorf("station correction differs from its persisted parent work or model route")
		}
		call = parent
	}
	if call.ID < 1 || call.ParentCallEvidenceID != 0 || len(call.WorkInput) == 0 {
		return queue.LLMCallEvidence{}, fmt.Errorf("station lineage has no initial work input")
	}
	return call, nil
}
