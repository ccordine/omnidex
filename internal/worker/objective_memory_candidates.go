package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
)

func (provider runtimeConversationCandidateProvider) MemoryCandidates(
	ctx context.Context,
	job model.Job,
) (objectiveMemoryContextCandidateSet, error) {
	if provider.runtime == nil || provider.runtime.svc == nil ||
		provider.runtime.svc.repo == nil {
		return objectiveMemoryContextCandidateSet{}, fmt.Errorf(
			"memory context retrieval requires repository authority",
		)
	}
	continuity, err := provider.runtime.svc.repo.ObjectiveContinuityAuthorities(ctx, job)
	if err != nil {
		return objectiveMemoryContextCandidateSet{}, err
	}
	set := objectiveMemoryContextCandidateSet{Replan: cloneObjectiveReplanAuthority(continuity.Replan)}
	if continuity.Scope == nil {
		return set, nil
	}
	hasMemory, err := provider.runtime.svc.repo.HasScopedMemory(ctx, *continuity.Scope)
	if err != nil {
		return objectiveMemoryContextCandidateSet{}, err
	}
	if !hasMemory {
		return set, nil
	}
	if provider.runtime.svc.embeddings == nil {
		return objectiveMemoryContextCandidateSet{}, fmt.Errorf(
			"scoped memory context retrieval requires embedding authority",
		)
	}
	embedding, err := provider.runtime.svc.embeddings.Embedding(ctx, job.Instruction)
	if err != nil {
		return objectiveMemoryContextCandidateSet{}, fmt.Errorf("embed exact objective turn for memory retrieval: %w", err)
	}
	matches, err := provider.runtime.svc.repo.FindRelevantMemory(
		ctx, *continuity.Scope, embedding, assemblyline.MaxMemoryContextCandidateAuthorities,
	)
	if err != nil {
		return objectiveMemoryContextCandidateSet{}, err
	}
	set.Candidates = make([]assemblyline.MemoryContextCandidate, len(matches))
	for index, match := range matches {
		if match.Scope != *continuity.Scope {
			return objectiveMemoryContextCandidateSet{}, fmt.Errorf("memory retrieval escaped its exact project/channel scope")
		}
		set.Candidates[index] = assemblyline.MemoryContextCandidate{
			MemoryID: match.ID, Kind: match.Kind, Content: match.Content,
			ContentSHA256: assemblyline.ExactObjectiveContextSHA(match.Content),
		}
	}
	return set, nil
}
