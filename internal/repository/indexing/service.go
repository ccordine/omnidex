package indexing

import (
	"context"
	"fmt"
	"strings"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	golangadapter "github.com/gryph/omnidex/internal/repository/adapters/golang"
)

type Store interface {
	StoreRepositorySnapshot(context.Context, int64, repositoryfacts.Snapshot) error
	StoreRepositoryAnalysis(context.Context, int64, repositoryfacts.Snapshot, repositoryfacts.Analysis) error
}

type Result struct {
	Snapshot repositoryfacts.Snapshot
	Analyses []repositoryfacts.Analysis
	Complete bool
}

type analyzer struct {
	identity repositoryfacts.AdapterIdentity
	analyze  func(context.Context, repositoryfacts.Snapshot) (repositoryfacts.Analysis, error)
}

type Service struct {
	store     Store
	analyzers map[string]analyzer
}

func New(store Store) (*Service, error) {
	if store == nil {
		return nil, fmt.Errorf("repository indexing requires durable storage")
	}
	return &Service{
		store: store,
		analyzers: map[string]analyzer{
			golangadapter.AdapterName: {
				identity: repositoryfacts.AdapterIdentity{
					Name: golangadapter.AdapterName, Version: golangadapter.AdapterVersion,
				},
				analyze: golangadapter.Analyze,
			},
		},
	}, nil
}

// Capture records exact repository authority without invoking an artifact
// analyzer. Analyzer selection is legal only after code has resolved one exact
// artifact demand and calls Analyze explicitly.
func (service *Service) Capture(ctx context.Context, projectID int64, root string) (Result, error) {
	if ctx == nil {
		return Result{}, fmt.Errorf("repository indexing requires a context")
	}
	if projectID < 1 {
		return Result{}, fmt.Errorf("repository indexing requires a positive project ID")
	}
	if service == nil || service.store == nil {
		return Result{}, fmt.Errorf("repository indexing service is unavailable")
	}
	snapshot, err := repositoryfacts.BuildGitSnapshot(ctx, root, repositoryfacts.SnapshotOptions{})
	if err != nil {
		return Result{}, err
	}
	result := Result{
		Snapshot: snapshot, Analyses: []repositoryfacts.Analysis{},
	}
	if err := service.store.StoreRepositorySnapshot(ctx, projectID, snapshot); err != nil {
		return result, err
	}
	result.Complete = true
	return result, nil
}

// Analyze runs exactly one code-demanded adapter against one already captured
// immutable snapshot. Other languages and analyzers have no bearing on this
// result.
func (service *Service) Analyze(
	ctx context.Context,
	projectID int64,
	snapshot repositoryfacts.Snapshot,
	adapterID string,
) (repositoryfacts.Analysis, error) {
	if ctx == nil {
		return repositoryfacts.Analysis{}, fmt.Errorf("repository analysis requires a context")
	}
	if projectID < 1 {
		return repositoryfacts.Analysis{}, fmt.Errorf("repository analysis requires a positive project ID")
	}
	if service == nil || service.store == nil || service.analyzers == nil {
		return repositoryfacts.Analysis{}, fmt.Errorf("repository analysis service is unavailable")
	}
	if err := snapshot.Validate(); err != nil {
		return repositoryfacts.Analysis{}, fmt.Errorf("repository analysis snapshot: %w", err)
	}
	if adapterID == "" || adapterID != strings.TrimSpace(adapterID) {
		return repositoryfacts.Analysis{}, fmt.Errorf("repository analysis requires one canonical adapter ID")
	}
	adapter, exists := service.analyzers[adapterID]
	if !exists {
		return repositoryfacts.Analysis{}, fmt.Errorf(
			"repository analyzer %q is not registered", adapterID,
		)
	}
	analysis, err := adapter.analyze(ctx, snapshot)
	if err != nil {
		return repositoryfacts.Analysis{}, fmt.Errorf("%s analysis failed: %w", adapterID, err)
	}
	if analysis.Adapter != adapter.identity {
		return repositoryfacts.Analysis{}, fmt.Errorf(
			"%s analysis returned adapter authority %+v; expected %+v",
			adapterID, analysis.Adapter, adapter.identity,
		)
	}
	if err := analysis.Validate(snapshot); err != nil {
		return repositoryfacts.Analysis{}, fmt.Errorf("%s analysis is invalid: %w", adapterID, err)
	}
	if err := service.store.StoreRepositoryAnalysis(ctx, projectID, snapshot, analysis); err != nil {
		return analysis, fmt.Errorf("store %s repository analysis: %w", adapterID, err)
	}
	if !analysis.Complete {
		return analysis, fmt.Errorf(
			"%s analysis is incomplete with %d diagnostics",
			adapterID, len(analysis.Diagnostics),
		)
	}
	return analysis, nil
}
