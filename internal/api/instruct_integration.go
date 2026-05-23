package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

type instructIntegrationRequest struct {
	Action      string          `json:"action"`
	Instruction string          `json:"instruction,omitempty"`
	Pipeline    string          `json:"pipeline,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
}

type instructIntegrationResult struct {
	Action  string     `json:"action"`
	Status  string     `json:"status"`
	Message string     `json:"message"`
	Job     *model.Job `json:"job,omitempty"`
}

type enqueueJobFunc func(ctx context.Context, instruction, pipeline string, metadata json.RawMessage) (model.Job, error)

type instructIntegrationService struct {
	enqueue enqueueJobFunc
}

func newInstructIntegrationService(repo *queue.Repository) *instructIntegrationService {
	service := &instructIntegrationService{}
	if repo == nil {
		return service
	}

	service.enqueue = func(ctx context.Context, instruction, pipeline string, metadata json.RawMessage) (model.Job, error) {
		return repo.EnqueueJob(ctx, instruction, pipeline, metadata)
	}

	return service
}

func (s *instructIntegrationService) Handle(ctx context.Context, req personaRequest) (instructIntegrationResult, bool, int, error) {
	if req.Integration == nil {
		return instructIntegrationResult{}, false, http.StatusOK, nil
	}

	action := normalizeIntegrationAction(req.Integration.Action)
	if action == "" {
		return instructIntegrationResult{}, false, http.StatusBadRequest, fmt.Errorf("integration.action is required when integration is provided")
	}

	switch action {
	case "enqueue_job":
		if s.enqueue == nil {
			return instructIntegrationResult{}, false, http.StatusServiceUnavailable, fmt.Errorf("job queue integration is unavailable")
		}

		instruction := strings.TrimSpace(req.Integration.Instruction)
		if instruction == "" {
			instruction = strings.TrimSpace(req.Prompt)
		}
		if instruction == "" {
			return instructIntegrationResult{}, false, http.StatusBadRequest, fmt.Errorf("integration requires a non-empty instruction or prompt")
		}

		pipeline, err := normalizeIntegrationPipeline(req.Integration.Pipeline)
		if err != nil {
			return instructIntegrationResult{}, false, http.StatusBadRequest, err
		}

		metadata, err := normalizeIntegrationMetadata(req.Integration.Metadata)
		if err != nil {
			return instructIntegrationResult{}, false, http.StatusBadRequest, err
		}

		job, err := s.enqueue(ctx, instruction, pipeline, metadata)
		if err != nil {
			return instructIntegrationResult{}, false, http.StatusInternalServerError, err
		}

		return instructIntegrationResult{
			Action:  "enqueue_job",
			Status:  "queued",
			Message: fmt.Sprintf("Queued job #%d using pipeline %s.", job.ID, pipeline),
			Job:     &job,
		}, true, http.StatusAccepted, nil
	default:
		return instructIntegrationResult{}, false, http.StatusBadRequest, fmt.Errorf("unsupported integration.action %q (supported: enqueue_job)", req.Integration.Action)
	}
}

func normalizeIntegrationAction(action string) string {
	normalized := strings.ToLower(strings.TrimSpace(action))
	switch normalized {
	case "enqueue_job", "queue_job", "enqueue_task", "job", "task":
		return "enqueue_job"
	default:
		return normalized
	}
}

func normalizeIntegrationPipeline(pipeline string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(pipeline)) {
	case "", model.PipelineAssistant:
		return model.PipelineAssistant, nil
	case model.PipelineChat:
		return model.PipelineChat, nil
	case model.PipelineCoding:
		return model.PipelineCoding, nil
	case model.PipelineStory:
		return model.PipelineStory, nil
	default:
		return "", fmt.Errorf("integration.pipeline must be assistant|chat|coding|story")
	}
}

func normalizeIntegrationMetadata(raw json.RawMessage) (json.RawMessage, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return json.RawMessage(`{}`), nil
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("integration.metadata must be valid JSON")
	}

	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("integration.metadata decode failed: %w", err)
	}

	if decoded == nil {
		return json.RawMessage(`{}`), nil
	}

	if _, ok := decoded.(map[string]any); !ok {
		return nil, fmt.Errorf("integration.metadata must be a JSON object")
	}

	normalized, err := json.Marshal(decoded)
	if err != nil {
		return nil, fmt.Errorf("integration.metadata encode failed: %w", err)
	}
	return json.RawMessage(normalized), nil
}
