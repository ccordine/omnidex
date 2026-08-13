package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
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

func (c *Client) EnqueueCoding(ctx context.Context, instruction string, metadata map[string]any) (model.Job, error) {
	if strings.TrimSpace(instruction) == "" {
		return model.Job{}, fmt.Errorf("coding instruction is required")
	}
	payload := map[string]any{
		"instruction": instruction,
		"pipeline":    model.PipelineCoding,
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
	if resp.Job.ID < 1 || resp.Job.Pipeline != model.PipelineCoding || resp.Job.Instruction != instruction {
		return model.Job{}, fmt.Errorf("coding enqueue returned a mismatched authoritative job")
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

func (c *Client) SubmitFeedback(ctx context.Context, id int64, operationID queue.LifecycleOperationID, feedback string) (model.Job, error) {
	payload := map[string]any{
		"operation_id": operationID,
		"feedback":     feedback,
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

func (c *Client) Interrupt(ctx context.Context, id int64, operationID queue.LifecycleOperationID, feedback string) (model.Job, error) {
	payload := map[string]any{
		"operation_id": operationID,
		"feedback":     feedback,
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

func (c *Client) Cancel(ctx context.Context, command queue.CancelJobCommand) (model.Job, error) {
	payload := map[string]any{
		"operation_id": command.OperationID,
		"reason":       command.Reason,
	}

	var resp struct {
		Job   model.Job `json:"job"`
		Error string    `json:"error"`
	}
	if err := c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/v1/jobs/%d/cancel", command.JobID), payload, &resp); err != nil {
		return model.Job{}, err
	}
	if resp.Error != "" {
		return model.Job{}, errors.New(resp.Error)
	}
	return resp.Job, nil
}

func (c *Client) Replan(ctx context.Context, id int64, operationID queue.LifecycleOperationID, feedback string) (model.Job, error) {
	payload := map[string]any{
		"operation_id": operationID,
		"feedback":     feedback,
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

func (c *Client) PromoteMemoryCandidate(
	ctx context.Context,
	id int64,
	tier string,
	authority model.MemoryPromotionAuthority,
) (model.MemoryCandidatePromotionResult, error) {
	if tier != model.MemoryCandidateStatusApproved && tier != model.MemoryCandidateStatusDurable {
		return model.MemoryCandidatePromotionResult{}, fmt.Errorf("memory promotion tier %q is not registered exact text", tier)
	}
	if authority != model.MemoryPromotionAuthorityCurrent &&
		authority != model.MemoryPromotionAuthorityHistorical &&
		authority != model.MemoryPromotionAuthorityGlobal {
		return model.MemoryCandidatePromotionResult{}, fmt.Errorf("memory promotion authority %q is not registered exact text", authority)
	}
	payload := map[string]any{
		"tier":      tier,
		"authority": authority,
	}
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

func (c *Client) MetricsRaw(ctx context.Context, path string) (json.RawMessage, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "/v1/metrics/live"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	var raw json.RawMessage
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}
