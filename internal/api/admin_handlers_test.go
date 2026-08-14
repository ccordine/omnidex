package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleIngestDocumentsRequiresFiles(t *testing.T) {
	s := &Server{repo: nil}
	req := httptest.NewRequest(http.MethodPost, "/v1/ingest/documents", nil)
	rec := httptest.NewRecorder()
	s.handleIngestDocuments(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 without repo, got %d", rec.Code)
	}
}

func TestHandleMindStatsRequiresRepo(t *testing.T) {
	s := &Server{repo: nil}
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/mind/stats", nil)
	rec := httptest.NewRecorder()
	s.handleMindStats(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestIngestMultipartMissingFiles(t *testing.T) {
	s := &Server{}
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/v1/ingest/documents", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	s.handleIngestDocuments(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleOllamaModelsMethodNotAllowed(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPut, "/v1/ollama/models", nil)
	rec := httptest.NewRecorder()
	s.handleOllamaModels(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestPullOllamaModelValidation(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/v1/ollama/models", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()
	s.handleOllamaModels(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &payload)
}

func TestOllamaPullRequestRejectsLooseOrUnboundedAuthority(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/v1/ollama/models", bytes.NewBufferString(`{"model":"registry.ollama.ai/library/qwen3:4b"}`))
	response := httptest.NewRecorder()
	decoded, err := decodeOllamaPullRequest(response, request)
	if err != nil || decoded.Model != "registry.ollama.ai/library/qwen3:4b" {
		t.Fatalf("decoded=%+v err=%v", decoded, err)
	}
	for _, body := range [][]byte{
		[]byte(`{"model":"one","model":"two"}`),
		[]byte(`{"model":"one","stream":true}`),
		[]byte(`{"Model":"one"}`),
		[]byte(`{"model":"one"} {}`),
		[]byte(`{"model":null}`),
		[]byte(`{"model":" padded "}`),
		[]byte(`{"model":"bad model"}`),
		bytes.Repeat([]byte(" "), int(maxOllamaPullBodyBytes+1)),
	} {
		request := httptest.NewRequest(http.MethodPost, "/v1/ollama/models", bytes.NewReader(body))
		response := httptest.NewRecorder()
		if _, err := decodeOllamaPullRequest(response, request); err == nil {
			t.Fatalf("invalid Ollama pull authority accepted: %q", body)
		}
	}
}
