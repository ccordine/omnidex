package worker

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/evidence"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	repositoryindex "github.com/gryph/omnidex/internal/repository/indexing"
)

type repositoryIndexService interface {
	Capture(context.Context, int64, string) (repositoryindex.Result, error)
	Analyze(
		context.Context,
		int64,
		repositoryfacts.Snapshot,
		string,
	) (repositoryfacts.Analysis, error)
}

func (r *nativeRuntimeV3) captureExistingRepositoryIndex(root string) (repositoryindex.Result, error) {
	if r == nil || r.svc == nil || r.svc.repo == nil || r.claim == nil {
		return repositoryindex.Result{}, fmt.Errorf("existing repository capture requires a claimed job")
	}
	if r.svc.repositoryIndex == nil {
		return repositoryindex.Result{}, fmt.Errorf("existing repository capture is unavailable")
	}
	projectID, err := r.svc.repo.JobProjectID(r.ctx, r.claim.Job.ID)
	if err != nil {
		return repositoryindex.Result{}, fmt.Errorf("resolve repository project authority: %w", err)
	}
	if projectID < 1 {
		return repositoryindex.Result{}, fmt.Errorf("existing repository capture requires durable project authority")
	}
	r.svc.emitStepEvent(r.claim.Authority, "repository_snapshot_started", "authority=server")
	result, err := r.svc.repositoryIndex.Capture(r.ctx, projectID, root)
	if err != nil {
		r.svc.emitStepEvent(r.claim.Authority, "repository_snapshot_failed", trimForBudget(err.Error(), 1000))
		return result, err
	}
	if !result.Complete {
		return result, fmt.Errorf("repository snapshot %q returned an incomplete result without an error", result.Snapshot.ID)
	}
	if err := result.Snapshot.Validate(); err != nil {
		return result, fmt.Errorf("captured repository snapshot is invalid: %w", err)
	}
	if len(result.Analyses) != 0 {
		return result, fmt.Errorf(
			"repository capture invoked %d analyzers before an artifact demand was resolved",
			len(result.Analyses),
		)
	}
	if err := r.writeEvidence(evidence.Record{
		Kind:       evidence.KindRepositoryIndex,
		SourceType: "repository",
		SourceRef:  result.Snapshot.ID,
		Hash:       result.Snapshot.GitStateSHA256,
		Summary:    "Repository snapshot ready without artifact analysis.",
		Confidence: 1,
		Metadata: map[string]any{
			"snapshot_id": result.Snapshot.ID,
			"head_commit": result.Snapshot.HeadCommit,
			"dirty":       result.Snapshot.Dirty,
			"file_count":  len(result.Snapshot.Files),
		},
	}); err != nil {
		return result, fmt.Errorf("record repository snapshot evidence: %w", err)
	}
	r.svc.emitStepEvent(r.claim.Authority, "repository_snapshot_ready", strings.Join([]string{
		"snapshot=" + result.Snapshot.ID,
		fmt.Sprintf("files=%d", len(result.Snapshot.Files)),
	}, " "))
	return result, nil
}

func (r *nativeRuntimeV3) demandExistingRepositoryAnalysis(
	snapshot repositoryfacts.Snapshot,
	adapterID string,
) (repositoryfacts.Analysis, error) {
	if r == nil || r.svc == nil || r.svc.repo == nil || r.claim == nil {
		return repositoryfacts.Analysis{}, fmt.Errorf("repository analysis demand requires a claimed job")
	}
	if r.svc.repositoryIndex == nil {
		return repositoryfacts.Analysis{}, fmt.Errorf("repository analysis demand is unavailable")
	}
	if err := snapshot.Validate(); err != nil {
		return repositoryfacts.Analysis{}, fmt.Errorf("repository analysis demand snapshot: %w", err)
	}
	if adapterID == "" || adapterID != strings.TrimSpace(adapterID) {
		return repositoryfacts.Analysis{}, fmt.Errorf("repository analysis demand requires one canonical adapter ID")
	}
	projectID, err := r.svc.repo.JobProjectID(r.ctx, r.claim.Job.ID)
	if err != nil {
		return repositoryfacts.Analysis{}, fmt.Errorf("resolve repository project authority: %w", err)
	}
	if projectID < 1 {
		return repositoryfacts.Analysis{}, fmt.Errorf("repository analysis demand requires durable project authority")
	}
	r.svc.emitStepEvent(
		r.claim.Authority, "repository_analysis_started",
		"snapshot="+snapshot.ID+" adapter="+adapterID,
	)
	analysis, err := r.svc.repositoryIndex.Analyze(
		r.ctx, projectID, snapshot, adapterID,
	)
	if err != nil {
		r.svc.emitStepEvent(
			r.claim.Authority, "repository_analysis_failed",
			trimForBudget(err.Error(), 1000),
		)
		return analysis, err
	}
	if !analysis.Complete {
		return analysis, fmt.Errorf(
			"repository analyzer %q returned an incomplete result without an error",
			adapterID,
		)
	}
	if err := analysis.Validate(snapshot); err != nil {
		return analysis, fmt.Errorf("demanded repository analysis is invalid: %w", err)
	}
	if err := r.writeEvidence(evidence.Record{
		Kind:       evidence.KindRepositoryIndex,
		SourceType: "repository",
		SourceRef:  analysis.ID,
		Hash:       snapshot.GitStateSHA256,
		Summary:    fmt.Sprintf("Repository analysis ready for adapter %s.", adapterID),
		Confidence: 1,
		Metadata: map[string]any{
			"snapshot_id": snapshot.ID,
			"analysis_id": analysis.ID,
			"adapter_id":  adapterID,
		},
	}); err != nil {
		return analysis, fmt.Errorf("record repository analysis evidence: %w", err)
	}
	r.svc.emitStepEvent(
		r.claim.Authority, "repository_analysis_ready",
		"snapshot="+snapshot.ID+" adapter="+adapterID+" analysis="+analysis.ID,
	)
	return analysis, nil
}
