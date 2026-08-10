package worker

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/evidence"
	repositoryindex "github.com/gryph/omnidex/internal/repository/indexing"
)

type repositoryIndexRefresher interface {
	Refresh(context.Context, int64, string) (repositoryindex.Result, error)
}

func (r *nativeRuntimeV3) refreshExistingRepositoryIndex(root string) (repositoryindex.Result, error) {
	if r == nil || r.svc == nil || r.svc.repo == nil || r.claim == nil {
		return repositoryindex.Result{}, fmt.Errorf("existing repository indexing requires a claimed job")
	}
	if r.svc.repositoryIndex == nil {
		return repositoryindex.Result{}, fmt.Errorf("existing repository indexing is unavailable")
	}
	projectID, err := r.svc.repo.JobProjectID(r.ctx, r.claim.Job.ID)
	if err != nil {
		return repositoryindex.Result{}, fmt.Errorf("resolve repository project authority: %w", err)
	}
	if projectID < 1 {
		return repositoryindex.Result{}, fmt.Errorf("existing repository indexing requires durable project authority")
	}
	r.svc.emitStepEvent(r.claim.Authority, "repository_index_started", "authority=server")
	result, err := r.svc.repositoryIndex.Refresh(r.ctx, projectID, root)
	if err != nil {
		r.svc.emitStepEvent(r.claim.Authority, "repository_index_failed", trimForBudget(err.Error(), 1000))
		return result, err
	}
	if !result.Complete {
		return result, fmt.Errorf("repository index %q returned an incomplete result without an error", result.Snapshot.ID)
	}
	analysisIDs := make([]string, 0, len(result.Analyses))
	for _, analysis := range result.Analyses {
		analysisIDs = append(analysisIDs, analysis.ID)
	}
	if err := r.writeEvidence(evidence.Record{
		Kind:       evidence.KindRepositoryIndex,
		SourceType: "repository",
		SourceRef:  result.Snapshot.ID,
		Hash:       result.Snapshot.GitStateSHA256,
		Summary:    fmt.Sprintf("Repository index ready with %d immutable analyses.", len(analysisIDs)),
		Confidence: 1,
		Metadata: map[string]any{
			"snapshot_id":  result.Snapshot.ID,
			"analysis_ids": analysisIDs,
			"head_commit":  result.Snapshot.HeadCommit,
			"dirty":        result.Snapshot.Dirty,
			"file_count":   len(result.Snapshot.Files),
		},
	}); err != nil {
		return result, fmt.Errorf("record repository index evidence: %w", err)
	}
	r.svc.emitStepEvent(r.claim.Authority, "repository_index_ready", strings.Join([]string{
		"snapshot=" + result.Snapshot.ID,
		fmt.Sprintf("files=%d", len(result.Snapshot.Files)),
		fmt.Sprintf("analyses=%d", len(result.Analyses)),
	}, " "))
	return result, nil
}
