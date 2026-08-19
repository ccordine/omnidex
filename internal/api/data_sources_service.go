package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/datasource"
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
		request, err := dataSourcePageRequest(r, dataSourceAPIPageSize)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		page, err := s.repo.ListDataSourcesPage(r.Context(), request)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"sources": dataSourcesPublicList(page.Items), "offset": page.Offset, "has_more": page.HasMore,
			"next_offset": dataSourceNextOffset(page.Offset, len(page.Items), page.HasMore),
		})
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
	updated, err := s.repo.UpdateDataSourceTestResult(r.Context(), id, status, message)
	if err != nil {
		writeDataSourceError(w, err)
		return
	}
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
	writeRemovedInferenceAction(w, "data-source natural-language query")
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
	snapshot, ok, err := s.repo.GetDataSourceSchemaSnapshot(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"source":          dataSourcePublic(record),
		"schema_snapshot": snapshot,
		"ready":           ok && len(snapshot.Relations) > 0,
	})
}

func (s *Server) handleDataSourceExplore(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeRemovedInferenceAction(w, "data-source inferred schema exploration")
}

func decodeDataSourceUpsert(r *http.Request) (queue.DataSourceUpsert, error) {
	var req struct {
		Name         string `json:"name"`
		Driver       string `json:"driver"`
		Host         string `json:"host"`
		Port         int    `json:"port"`
		DatabaseName string `json:"database_name"`
		Username     string `json:"username"`
		Password     string `json:"password"`
		SSLMode      string `json:"ssl_mode"`
		UseDSN       bool   `json:"use_dsn"`
		DSN          string `json:"dsn"`
	}
	decoder := json.NewDecoder(io.LimitReader(r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		return queue.DataSourceUpsert{}, fmt.Errorf("invalid data-source body: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return queue.DataSourceUpsert{}, fmt.Errorf("data-source body must contain exactly one JSON object")
	}
	return queue.DataSourceUpsert{
		Name: req.Name, Driver: req.Driver, Host: req.Host, Port: req.Port,
		DatabaseName: req.DatabaseName, Username: req.Username, Password: req.Password,
		SSLMode: req.SSLMode, UseDSN: req.UseDSN, DSN: req.DSN,
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
