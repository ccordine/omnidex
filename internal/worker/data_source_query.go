package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/datasource"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/omni"
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
	llm, err := s.dataSourceLLMClient()
	if err != nil {
		return err
	}

	s.emitStepEvent(claim.Step.ID, "data_source_query_started", record.Name)

	catalog, hasCatalog, _ := s.repo.GetDataSourceCatalog(ctx, sourceID)
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
		_ = store.Save(ctx, updatedCatalog)
		_ = s.repo.UpdateDataSourceCatalogTimestamp(ctx, record.ID, updatedCatalog.UpdatedAt)
		_ = datasource.PersistCatalogMemories(ctx, writer, updatedCatalog)
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
		_, _ = s.repo.AddDataSourceChannelMessage(ctx, channelID, "assistant", summary, payloadBytes, &jobID)
	}
	completeStep := s.completeStep
	if completeStep == nil {
		completeStep = s.repo.CompleteStep
	}
	s.emitStepEvent(claim.Step.ID, "data_source_query_completed", summary)
	return completeStep(ctx, claim.Step.ID, string(payloadBytes), "data_source_query", summary)
}

func (s *Service) dataSourceLLMClient() (omni.DBManagerLLMClient, error) {
	endpoint := strings.TrimRight(strings.TrimSpace(s.ollamaBaseURL), "/")
	if endpoint == "" {
		return nil, fmt.Errorf("ollama endpoint is not configured for data source queries")
	}
	modelName := firstNonEmptyString(s.models.Tagging, s.models.Default, "qwen3:4b-thinking")
	return omni.NewOllamaClient(endpoint+"/api/chat", modelName), nil
}
