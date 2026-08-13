package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
)

func resolveObjectiveMemoryContext(
	ctx context.Context,
	job model.Job,
	authority turnAuthority,
	provider objectiveMemoryContextCandidateProvider,
	selector objectiveMemoryContextSelectionStation,
) (turnAuthority, int, error) {
	if provider == nil {
		return turnAuthority{}, 0, fmt.Errorf("memory context candidate authority provider is unavailable")
	}
	set, err := provider.MemoryCandidates(ctx, job)
	if err != nil {
		return turnAuthority{}, 0, err
	}
	authority.Context.ReplanAuthority = cloneObjectiveReplanAuthority(set.Replan)
	if set.Replan != nil && (set.Replan.JobID != job.ID ||
		set.Replan.Generation != job.CurrentGeneration) {
		return turnAuthority{}, 0, fmt.Errorf(
			"objective replan authority must match current generation %d of job %d",
			job.CurrentGeneration, job.ID,
		)
	}
	set.Candidates = append([]assemblyline.MemoryContextCandidate(nil), set.Candidates...)
	if err := authority.Context.Validate(); err != nil {
		return turnAuthority{}, 0, err
	}
	if len(set.Candidates) == 0 {
		return authority, 0, nil
	}
	input := assemblyline.MemoryContextSelectionInput{
		ExactInstruction:     authority.Instruction,
		MaxSelectedBytes:     assemblyline.MaxSelectedMemoryProjectionBytes,
		CandidateAuthorities: set.Candidates,
	}
	if _, err := assemblyline.NewMemoryContextSelectionJob(input); err != nil {
		return turnAuthority{}, 0, err
	}
	if selector == nil {
		return turnAuthority{}, 0, fmt.Errorf("memory context-selection station is unavailable")
	}
	decision, receipt, err := selector.SelectMemory(ctx, input)
	if err != nil {
		return turnAuthority{}, 0, err
	}
	if receipt.Calls < 1 || receipt.Calls > maxTypedWorkerAttempts {
		return turnAuthority{}, 0, fmt.Errorf(
			"memory context-selection station reported %d calls outside the bounded correction budget",
			receipt.Calls,
		)
	}
	if err := decision.ValidateFor(input); err != nil {
		return turnAuthority{}, 0, err
	}
	selected := make(map[int64]struct{}, len(decision.ReferencedMemoryIDs))
	for _, id := range decision.ReferencedMemoryIDs {
		selected[id] = struct{}{}
	}
	for _, candidate := range set.Candidates {
		if _, exists := selected[candidate.MemoryID]; !exists {
			continue
		}
		authority.Context.MemoryAuthorities = append(
			authority.Context.MemoryAuthorities,
			assemblyline.ObjectiveMemoryAuthority{
				MemoryID: candidate.MemoryID, Kind: candidate.Kind,
				Content: candidate.Content, ContentSHA256: candidate.ContentSHA256,
			},
		)
	}
	if err := authority.Context.Validate(); err != nil {
		return turnAuthority{}, 0, err
	}
	return authority, receipt.Calls, nil
}

func cloneObjectiveReplanAuthority(
	value *assemblyline.ObjectiveReplanAuthority,
) *assemblyline.ObjectiveReplanAuthority {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
