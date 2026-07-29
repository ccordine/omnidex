package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/datasource"
	"github.com/gryph/omnidex/internal/model"
)

func (s *Service) runDataSourceQueryStep(ctx context.Context, claim *model.ClaimedStep) error {
	if s.repo == nil {
		return fmt.Errorf("data source query requires repository")
	}
	sourceID, _, question, err := datasource.ParseJobMetadata(claim.Job.Metadata)
	if err != nil {
		return err
	}
	record, err := s.repo.GetDataSource(ctx, sourceID)
	if err != nil {
		return fmt.Errorf("load data source: %w", err)
	}
	llm, err := s.dataSourceLLMClient(claim.Job)
	if err != nil {
		return err
	}

	s.emitStepEvent(claim.Step.ID, "data_source_query_started", record.Name)

	catalog, hasCatalog, err := s.repo.GetDataSourceCatalog(ctx, sourceID)
	if err != nil {
		return fmt.Errorf("load data source catalog: %w", err)
	}
	store := &repoCatalogStore{svc: s}
	writer := &dataSourceMemoryWriter{repo: s.repo}
	result, updatedCatalog, err := datasource.AnalyticalAsk(ctx, datasource.AnalyticalAskInput{
		Connection: record.Connection(),
		Profile:    record.Profile(),
		SourceID:   record.ID,
		SourceName: record.Name,
		Question:   question,
		Catalog:    catalog,
		HasCatalog: hasCatalog,
	}, llm)
	if err != nil {
		return err
	}
	if len(updatedCatalog.Tables) > 0 && (!hasCatalog || updatedCatalog.Fingerprint != catalog.Fingerprint) {
		updatedCatalog.UpdatedAt = time.Now().UTC()
		if err := store.Save(ctx, updatedCatalog); err != nil {
			return fmt.Errorf("save updated data source catalog: %w", err)
		}
		if err := s.repo.UpdateDataSourceCatalogTimestamp(ctx, record.ID, updatedCatalog.UpdatedAt); err != nil {
			return fmt.Errorf("update data source catalog timestamp: %w", err)
		}
		if err := datasource.PersistCatalogMemories(ctx, writer, updatedCatalog); err != nil {
			return fmt.Errorf("persist data source catalog memories: %w", err)
		}
	}
	summary, _, err := datasource.FormatJobResult(result)
	if err != nil {
		return err
	}
	channelPayload := datasource.BuildChannelMessagePayload(result, claim.Job.ID)
	payloadBytes, err := json.Marshal(channelPayload)
	if err != nil {
		return err
	}
	if channelID := datasource.ParseChannelID(claim.Job.Metadata); channelID != "" {
		jobID := claim.Job.ID
		if _, err := s.repo.AddDataSourceChannelMessage(ctx, channelID, "assistant", summary, payloadBytes, &jobID); err != nil {
			return fmt.Errorf("append data source channel result: %w", err)
		}
	}
	completeStep := s.completeStep
	if completeStep == nil {
		completeStep = s.repo.CompleteStep
	}
	s.emitStepEvent(claim.Step.ID, "data_source_query_completed", summary)
	return completeStep(ctx, claim.Step.ID, string(payloadBytes), "data_source_query", summary)
}
