package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/datasource"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/secrets"
	"github.com/jackc/pgx/v5"
)

func (s *Server) handleDataSources(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	switch r.Method {
	case http.MethodGet:
		items, err := s.repo.ListDataSources(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"sources": dataSourcesPublicList(items)})
	case http.MethodPost:
		s.handleDataSourceCreate(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleDataSourceByID(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/admin/data-sources/"), "/")
	if id == "" {
		writeError(w, http.StatusNotFound, "data source not found")
		return
	}
	if strings.HasSuffix(id, "/test") {
		s.handleDataSourceTest(w, r, strings.TrimSuffix(id, "/test"))
		return
	}
	if strings.HasSuffix(id, "/schema") {
		s.handleDataSourceSchema(w, r, strings.TrimSuffix(id, "/schema"))
		return
	}
	if strings.HasSuffix(id, "/query") {
		s.handleDataSourceQuery(w, r, strings.TrimSuffix(id, "/query"))
		return
	}
	if strings.HasSuffix(id, "/ask") {
		s.handleDataSourceAsk(w, r, strings.TrimSuffix(id, "/ask"))
		return
	}
	if strings.HasSuffix(id, "/catalog") {
		s.handleDataSourceCatalog(w, r, strings.TrimSuffix(id, "/catalog"))
		return
	}
	if strings.HasSuffix(id, "/explore") {
		s.handleDataSourceExplore(w, r, strings.TrimSuffix(id, "/explore"))
		return
	}
	switch r.Method {
	case http.MethodPut:
		s.handleDataSourceUpdate(w, r, id)
	case http.MethodDelete:
		if err := s.repo.DeleteDataSource(r.Context(), id); err != nil {
			writeDataSourceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleDataSourceCreate(w http.ResponseWriter, r *http.Request) {
	input, err := decodeDataSourceUpsert(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	record, err := s.repo.CreateDataSource(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"source": dataSourcePublic(record)})
}

func (s *Server) handleDataSourceUpdate(w http.ResponseWriter, r *http.Request, id string) {
	input, err := decodeDataSourceUpsert(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	record, err := s.repo.UpdateDataSource(r.Context(), id, input)
	if err != nil {
		writeDataSourceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": dataSourcePublic(record)})
}

func (s *Server) handleDataSourceTest(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	record, err := s.repo.GetDataSource(r.Context(), id)
	if err != nil {
		writeDataSourceError(w, err)
		return
	}
	status, message, err := s.testDataSourceConnection(r.Context(), record)
	if err != nil {
		status = "failed"
		message = err.Error()
	}
	updated, _ := s.repo.UpdateDataSourceTestResult(r.Context(), id, status, message)
	writeJSON(w, http.StatusOK, map[string]any{
		"source":  dataSourcePublic(updated),
		"status":  status,
		"message": message,
	})
}

func (s *Server) handleDataSourceSchema(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	record, err := s.repo.GetDataSource(r.Context(), id)
	if err != nil {
		writeDataSourceError(w, err)
		return
	}
	schema, err := datasource.InspectSchema(r.Context(), record.Connection())
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"schema": schema})
}

func (s *Server) handleDataSourceQuery(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		SQL string `json:"sql"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	record, err := s.repo.GetDataSource(r.Context(), id)
	if err != nil {
		writeDataSourceError(w, err)
		return
	}
	result, err := datasource.RunSQL(r.Context(), record.Connection(), req.SQL)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"sql":     result.SQL,
		"columns": result.Columns,
		"rows":    result.Rows,
		"count":   result.Count,
	})
}

func (s *Server) handleDataSourceAsk(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Question string `json:"question"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	question := strings.TrimSpace(req.Question)
	if question == "" {
		writeError(w, http.StatusBadRequest, "question is required")
		return
	}
	record, err := s.repo.GetDataSource(r.Context(), id)
	if err != nil {
		writeDataSourceError(w, err)
		return
	}
	metadata, err := datasource.JobMetadata(record.ID, record.Name, question, "")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	job, err := s.repo.EnqueueJob(r.Context(), question, model.PipelineDataQuery, metadata)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"job":      job,
		"question": question,
		"message":  fmt.Sprintf("Queued data query job #%d", job.ID),
	})
}

func (s *Server) handleDataSourceCatalog(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	record, err := s.repo.GetDataSource(r.Context(), id)
	if err != nil {
		writeDataSourceError(w, err)
		return
	}
	catalog, ok, err := s.repo.GetDataSourceCatalog(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"source":  dataSourcePublic(record),
		"catalog": catalog,
		"ready":   ok && len(catalog.Tables) > 0,
	})
}

func (s *Server) handleDataSourceExplore(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	record, err := s.repo.GetDataSource(r.Context(), id)
	if err != nil {
		writeDataSourceError(w, err)
		return
	}
	metadata, err := datasource.ExploreJobMetadata(record.ID, record.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	job, err := s.repo.EnqueueJob(r.Context(), fmt.Sprintf("Explore schema map for %s", record.Name), model.PipelineDataExplore, metadata)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"job":     job,
		"message": fmt.Sprintf("Queued schema exploration job #%d", job.ID),
	})
}

func decodeDataSourceUpsert(r *http.Request) (queue.DataSourceUpsert, error) {
	var req struct {
		Name          string `json:"name"`
		Driver        string `json:"driver"`
		Domain        string `json:"domain"`
		ContextPrompt string `json:"context_prompt"`
		PrivacyMode   string `json:"privacy_mode"`
		Host          string `json:"host"`
		Port          int    `json:"port"`
		DatabaseName  string `json:"database_name"`
		Username      string `json:"username"`
		Password      string `json:"password"`
		SSLMode       string `json:"ssl_mode"`
		UseDSN        bool   `json:"use_dsn"`
		DSN           string `json:"dsn"`
		ReadOnly      *bool  `json:"read_only"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return queue.DataSourceUpsert{}, fmt.Errorf("invalid json body")
	}
	readOnly := true
	if req.ReadOnly != nil {
		readOnly = *req.ReadOnly
	}
	return queue.DataSourceUpsert{
		Name:          req.Name,
		Driver:        req.Driver,
		Domain:        req.Domain,
		ContextPrompt: req.ContextPrompt,
		PrivacyMode:   req.PrivacyMode,
		Host:          req.Host,
		Port:          req.Port,
		DatabaseName:  req.DatabaseName,
		Username:      req.Username,
		Password:      req.Password,
		SSLMode:       req.SSLMode,
		UseDSN:        req.UseDSN,
		DSN:           req.DSN,
		ReadOnly:      readOnly,
	}, nil
}

func (s *Server) testDataSourceConnection(ctx context.Context, record queue.DataSourceRecord) (string, string, error) {
	schema, err := datasource.InspectSchema(ctx, record.Connection())
	if err != nil {
		return "failed", "", err
	}
	return "ok", fmt.Sprintf("Connected read-only (%d tables)", len(schema)), nil
}

func dataSourcesPublicList(items []queue.DataSourceRecord) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, dataSourcePublic(item))
	}
	return out
}

func dataSourcePublic(record queue.DataSourceRecord) map[string]any {
	payload := map[string]any{
		"id":                record.ID,
		"name":              record.Name,
		"driver":            record.Driver,
		"domain":            record.Domain,
		"context_prompt":    record.ContextPrompt,
		"privacy_mode":      record.PrivacyMode,
		"host":              record.Host,
		"port":              record.Port,
		"database_name":     record.DatabaseName,
		"username":          record.Username,
		"ssl_mode":          record.SSLMode,
		"use_dsn":           record.UseDSN,
		"read_only":         record.ReadOnly,
		"password_set":      strings.TrimSpace(record.Password) != "" || strings.TrimSpace(record.DSN) != "",
		"password_hint":     dataSourceSecretHint(record),
		"last_test_status":  record.LastTestStatus,
		"last_test_message": record.LastTestMessage,
		"created_at":        record.CreatedAt,
		"updated_at":        record.UpdatedAt,
	}
	if record.LastTestAt != nil {
		payload["last_test_at"] = record.LastTestAt.UTC().Format(time.RFC3339)
	}
	if record.CatalogUpdatedAt != nil {
		payload["catalog_updated_at"] = record.CatalogUpdatedAt.UTC().Format(time.RFC3339)
	}
	return payload
}

func dataSourceSecretHint(record queue.DataSourceRecord) string {
	if record.UseDSN {
		return secrets.MaskHint(record.DSN)
	}
	return secrets.MaskHint(record.Password)
}

func writeDataSourceError(w http.ResponseWriter, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "data source not found")
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error())
}
