package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func New(baseURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) Enqueue(ctx context.Context, instruction, pipeline string, metadata map[string]any) (model.Job, error) {
	payload := map[string]any{
		"instruction": instruction,
		"pipeline":    pipeline,
		"metadata":    metadata,
	}

	var resp struct {
		Job   model.Job `json:"job"`
		Error string    `json:"error"`
	}

	if err := c.doJSON(ctx, http.MethodPost, "/v1/jobs", payload, &resp); err != nil {
		return model.Job{}, err
	}

	if resp.Error != "" {
		return model.Job{}, errors.New(resp.Error)
	}

	return resp.Job, nil
}

func (c *Client) List(ctx context.Context, status string, limit, offset int) ([]model.Job, error) {
	path := fmt.Sprintf("/v1/jobs?limit=%d&offset=%d", limit, offset)
	if status != "" {
		path += "&status=" + status
	}

	var resp struct {
		Jobs  []model.Job `json:"jobs"`
		Error string      `json:"error"`
	}
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, errors.New(resp.Error)
	}
	return resp.Jobs, nil
}

func (c *Client) Show(ctx context.Context, id int64) (model.JobDetails, error) {
	var details model.JobDetails
	if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/v1/jobs/%d", id), nil, &details); err != nil {
		return model.JobDetails{}, err
	}
	return details, nil
}

func (c *Client) Inspect(ctx context.Context, id int64) (model.JobInspection, error) {
	var inspection model.JobInspection
	if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/v1/jobs/%d/inspection", id), nil, &inspection); err != nil {
		return model.JobInspection{}, err
	}
	return inspection, nil
}

func (c *Client) SubmitFeedback(ctx context.Context, id int64, feedback string) (model.Job, error) {
	payload := map[string]any{
		"feedback": feedback,
	}

	var resp struct {
		Job   model.Job `json:"job"`
		Error string    `json:"error"`
	}
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/jobs/%d/feedback", id), payload, &resp); err != nil {
		return model.Job{}, err
	}
	if resp.Error != "" {
		return model.Job{}, errors.New(resp.Error)
	}
	return resp.Job, nil
}

func (c *Client) Interrupt(ctx context.Context, id int64, feedback string) (model.Job, error) {
	payload := map[string]any{
		"feedback": feedback,
	}

	var resp struct {
		Job   model.Job `json:"job"`
		Error string    `json:"error"`
	}
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/jobs/%d/interrupt", id), payload, &resp); err != nil {
		return model.Job{}, err
	}
	if resp.Error != "" {
		return model.Job{}, errors.New(resp.Error)
	}
	return resp.Job, nil
}

func (c *Client) Cancel(ctx context.Context, id int64, reason string) (model.Job, error) {
	payload := map[string]any{
		"reason": reason,
	}

	var resp struct {
		Job   model.Job `json:"job"`
		Error string    `json:"error"`
	}
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/jobs/%d/cancel", id), payload, &resp); err != nil {
		return model.Job{}, err
	}
	if resp.Error != "" {
		return model.Job{}, errors.New(resp.Error)
	}
	return resp.Job, nil
}

func (c *Client) Replan(ctx context.Context, id int64, feedback string) (model.Job, error) {
	payload := map[string]any{
		"feedback": feedback,
	}

	var resp struct {
		Job   model.Job `json:"job"`
		Error string    `json:"error"`
	}
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/jobs/%d/replan", id), payload, &resp); err != nil {
		return model.Job{}, err
	}
	if resp.Error != "" {
		return model.Job{}, errors.New(resp.Error)
	}
	return resp.Job, nil
}

func (c *Client) AddMemory(ctx context.Context, source, kind, content string, tags []string) (model.MemoryChunk, error) {
	payload := map[string]any{
		"source":  source,
		"kind":    kind,
		"content": content,
		"tags":    tags,
	}

	var resp struct {
		Memory model.MemoryChunk `json:"memory"`
		Error  string            `json:"error"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/v1/memory", payload, &resp); err != nil {
		return model.MemoryChunk{}, err
	}
	if resp.Error != "" {
		return model.MemoryChunk{}, errors.New(resp.Error)
	}
	return resp.Memory, nil
}

func (c *Client) ListMemoryCategories(ctx context.Context, limit int) ([]model.MemoryFacet, error) {
	if limit <= 0 {
		limit = 100
	}
	var resp struct {
		Categories []model.MemoryFacet `json:"categories"`
		Error      string              `json:"error"`
	}
	if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/v1/memory/categories?limit=%d", limit), nil, &resp); err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, errors.New(resp.Error)
	}
	return resp.Categories, nil
}

func (c *Client) ListMemoryTags(ctx context.Context, limit int) ([]model.MemoryFacet, error) {
	if limit <= 0 {
		limit = 100
	}
	var resp struct {
		Tags  []model.MemoryFacet `json:"tags"`
		Error string              `json:"error"`
	}
	if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/v1/memory/tags?limit=%d", limit), nil, &resp); err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, errors.New(resp.Error)
	}
	return resp.Tags, nil
}

type ResearchIngestRequest struct {
	Topic                  string   `json:"topic"`
	Source                 string   `json:"source,omitempty"`
	Kind                   string   `json:"kind,omitempty"`
	Tags                   []string `json:"tags,omitempty"`
	ChunkSize              int      `json:"chunk_size,omitempty"`
	Overlap                int      `json:"overlap,omitempty"`
	MaxChunks              int      `json:"max_chunks,omitempty"`
	IncludeOfficialSources *bool    `json:"include_official_sources,omitempty"`
}

type ResearchIngestResponse struct {
	Topic             string   `json:"topic"`
	Slug              string   `json:"slug"`
	SourcePrefix      string   `json:"source_prefix"`
	StoredChunks      int      `json:"stored_chunks"`
	Tags              []string `json:"tags"`
	Warnings          []string `json:"warnings,omitempty"`
	Dossier           string   `json:"dossier,omitempty"`
	Sources           []string `json:"sources,omitempty"`
	StoredChunkSource []string `json:"stored_chunk_sources,omitempty"`
}

func (c *Client) ResearchIngest(ctx context.Context, req ResearchIngestRequest) (ResearchIngestResponse, error) {
	var resp ResearchIngestResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v1/research/ingest", req, &resp); err != nil {
		return ResearchIngestResponse{}, err
	}
	return resp, nil
}

func (c *Client) ListMemoryCandidates(ctx context.Context, jobID int64, status string, limit int) ([]model.MemoryCandidate, error) {
	path := fmt.Sprintf("/v1/memory-candidates?limit=%d", limit)
	if jobID > 0 {
		path += fmt.Sprintf("&job_id=%d", jobID)
	}
	if strings.TrimSpace(status) != "" {
		path += "&status=" + strings.TrimSpace(status)
	}
	var resp struct {
		MemoryCandidates []model.MemoryCandidate `json:"memory_candidates"`
		Error            string                  `json:"error"`
	}
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, errors.New(resp.Error)
	}
	return resp.MemoryCandidates, nil
}

func (c *Client) PromoteMemoryCandidate(ctx context.Context, id int64, tier string) (model.MemoryCandidatePromotionResult, error) {
	payload := map[string]any{"tier": strings.TrimSpace(tier)}
	var resp model.MemoryCandidatePromotionResult
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/memory-candidates/%d/promote", id), payload, &resp); err != nil {
		return model.MemoryCandidatePromotionResult{}, err
	}
	return resp, nil
}

func (c *Client) RejectMemoryCandidate(ctx context.Context, id int64) (model.MemoryCandidate, error) {
	var resp struct {
		MemoryCandidate model.MemoryCandidate `json:"memory_candidate"`
		Error           string                `json:"error"`
	}
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/memory-candidates/%d/reject", id), map[string]any{}, &resp); err != nil {
		return model.MemoryCandidate{}, err
	}
	if resp.Error != "" {
		return model.MemoryCandidate{}, errors.New(resp.Error)
	}
	return resp.MemoryCandidate, nil
}

func (c *Client) MigrateFresh(ctx context.Context) error {
	var resp struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/v1/admin/migrate-fresh", map[string]any{}, &resp); err != nil {
		return err
	}
	if resp.Error != "" {
		return errors.New(resp.Error)
	}
	return nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errBody struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &errBody) == nil && errBody.Error != "" {
			return fmt.Errorf("%s", errBody.Error)
		}
		return fmt.Errorf("request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return err
		}
	}

	return nil
}
