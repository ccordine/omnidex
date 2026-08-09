package indexing

import (
	"context"
	"fmt"
	"sort"
	"strings"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	golangadapter "github.com/gryph/omnidex/internal/repository/adapters/golang"
)

type Store interface {
	StoreRepositorySnapshot(context.Context, int64, repositoryfacts.Snapshot) error
	StoreRepositoryAnalysis(context.Context, int64, repositoryfacts.Snapshot, repositoryfacts.Analysis) error
}

type Result struct {
	Snapshot             repositoryfacts.Snapshot
	Analyses             []repositoryfacts.Analysis
	UnsupportedLanguages []string
	Complete             bool
}

type analyzer struct {
	name      string
	languages []string
	analyze   func(context.Context, repositoryfacts.Snapshot) (repositoryfacts.Analysis, error)
}

type Service struct {
	store     Store
	analyzers []analyzer
}

func New(store Store) (*Service, error) {
	if store == nil {
		return nil, fmt.Errorf("repository indexing requires durable storage")
	}
	return &Service{
		store: store,
		analyzers: []analyzer{
			{name: golangadapter.AdapterName, languages: []string{"go"}, analyze: golangadapter.Analyze},
		},
	}, nil
}

func (service *Service) Refresh(ctx context.Context, projectID int64, root string) (Result, error) {
	if ctx == nil {
		return Result{}, fmt.Errorf("repository indexing requires a context")
	}
	if projectID < 1 {
		return Result{}, fmt.Errorf("repository indexing requires a positive project ID")
	}
	if service == nil || service.store == nil || len(service.analyzers) == 0 {
		return Result{}, fmt.Errorf("repository indexing service is unavailable")
	}
	snapshot, err := repositoryfacts.BuildGitSnapshot(ctx, root, repositoryfacts.SnapshotOptions{})
	if err != nil {
		return Result{}, err
	}
	result := Result{Snapshot: snapshot}
	if err := service.store.StoreRepositorySnapshot(ctx, projectID, snapshot); err != nil {
		return result, err
	}
	detected := sourceLanguages(snapshot)
	supported := make(map[string]struct{})
	problems := make([]string, 0)
	for _, adapter := range service.analyzers {
		if !adapterRequired(adapter, detected) {
			continue
		}
		for _, language := range adapter.languages {
			supported[language] = struct{}{}
		}
		analysis, analyzeErr := adapter.analyze(ctx, snapshot)
		if analyzeErr != nil {
			problems = append(problems, fmt.Sprintf("%s analysis failed: %v", adapter.name, analyzeErr))
			continue
		}
		if err := service.store.StoreRepositoryAnalysis(ctx, projectID, snapshot, analysis); err != nil {
			return result, fmt.Errorf("store %s repository analysis: %w", adapter.name, err)
		}
		result.Analyses = append(result.Analyses, analysis)
		if !analysis.Complete {
			problems = append(problems, fmt.Sprintf("%s analysis is incomplete with %d diagnostics", adapter.name, len(analysis.Diagnostics)))
		}
	}
	for _, language := range detected {
		if _, exists := supported[language]; !exists {
			result.UnsupportedLanguages = append(result.UnsupportedLanguages, language)
		}
	}
	if len(result.UnsupportedLanguages) > 0 {
		problems = append(problems, "unsupported source languages: "+strings.Join(result.UnsupportedLanguages, ", "))
	}
	if len(detected) == 0 {
		problems = append(problems, "no supported source files were found")
	}
	result.Complete = len(problems) == 0
	if !result.Complete {
		return result, fmt.Errorf("repository index incomplete: %s", strings.Join(problems, "; "))
	}
	return result, nil
}

func adapterRequired(adapter analyzer, detected []string) bool {
	for _, supported := range adapter.languages {
		for _, language := range detected {
			if supported == language {
				return true
			}
		}
	}
	return false
}

func sourceLanguages(snapshot repositoryfacts.Snapshot) []string {
	sourceKinds := map[string]struct{}{
		"go": {}, "typescript": {}, "javascript": {}, "php": {}, "python": {},
		"java": {}, "kotlin": {}, "rust": {},
	}
	found := make(map[string]struct{})
	for _, file := range snapshot.Files {
		if file.Kind != repositoryfacts.EntryRegular {
			continue
		}
		if _, source := sourceKinds[file.Language]; source {
			found[file.Language] = struct{}{}
		}
	}
	languages := make([]string, 0, len(found))
	for language := range found {
		languages = append(languages, language)
	}
	sort.Strings(languages)
	return languages
}
