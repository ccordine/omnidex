package worker

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/datasource"
	"github.com/gryph/omnidex/internal/model"
)

type repoCatalogStore struct {
	svc *Service
}

func (s *repoCatalogStore) Get(ctx context.Context, sourceID string) (datasource.SchemaCatalog, bool, error) {
	return s.svc.repo.GetDataSourceCatalog(ctx, sourceID)
}

func (s *repoCatalogStore) Save(ctx context.Context, catalog datasource.SchemaCatalog) error {
	return s.svc.repo.SaveDataSourceCatalog(ctx, catalog)
}

func (s *Service) runDataSourceExploreStep(ctx context.Context, claim *model.ClaimedStep) error {
	if s.repo == nil {
		return fmt.Errorf("data source explore requires repository")
	}
	sourceID, sourceName, err := datasource.ParseExploreMetadata(claim.Job.Metadata)
	if err != nil {
		return err
	}
	record, err := s.repo.GetDataSource(ctx, sourceID)
	if err != nil {
		return fmt.Errorf("load data source: %w", err)
	}
	llm, err := s.dataSourceLLMClient()
	if err != nil {
		return err
	}

	s.emitStepEvent(claim.Step.ID, "data_source_explore_started", record.Name)
	store := &repoCatalogStore{svc: s}
	writer := &dataSourceMemoryWriter{repo: s.repo}
	catalog, err := datasource.EnsureCatalog(ctx, store, writer, record.Connection(), record.Profile(), record.ID, sourceName, llm)
	if err != nil {
		return err
	}
	_ = s.repo.UpdateDataSourceCatalogTimestamp(ctx, record.ID, catalog.UpdatedAt)

	summary := fmt.Sprintf("Cataloged %d tables for %s (%s). %s", len(catalog.Tables), record.Name, record.Profile().Domain, catalog.Summary)
	payloadBytes, err := json.Marshal(map[string]any{
		"catalog": catalog,
		"summary": summary,
	})
	if err != nil {
		return err
	}
	completeStep := s.completeStep
	if completeStep == nil {
		completeStep = s.repo.CompleteStep
	}
	s.emitStepEvent(claim.Step.ID, "data_source_explore_completed", summary)
	return completeStep(ctx, claim.Step.ID, string(payloadBytes), "data_source_explore", summary)
}
