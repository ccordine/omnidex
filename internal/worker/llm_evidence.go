package worker

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

type llmEvidenceWork struct {
	ID   string
	Kind string
}

type llmEvidencePersistenceError struct {
	PersistenceError error
	CallError        error
}

func (e llmEvidencePersistenceError) Error() string {
	if e.CallError == nil {
		return fmt.Sprintf("persist exact LLM call evidence: %v", e.PersistenceError)
	}
	return fmt.Sprintf("persist exact LLM call evidence after call error %q: %v", e.CallError, e.PersistenceError)
}

func (e llmEvidencePersistenceError) Unwrap() error {
	return e.PersistenceError
}

func (s *Service) llmGeneratePortableWithSchemaTrace(
	ctx context.Context,
	stepID int64,
	job assemblyline.PortableJob,
	scope, modelName, prompt string,
	responseSchema map[string]any,
) (string, error) {
	work, err := llmEvidenceWorkForPortableJob(job)
	if err != nil {
		return "", err
	}
	return s.llmGenerateWithEvidenceTrace(ctx, stepID, scope, modelName, prompt, responseSchema, work)
}

func (s *Service) llmGeneratePortableAdvisoryTrace(
	ctx context.Context,
	stepID int64,
	job assemblyline.PortableJob,
	modelName, prompt string,
) (llm.AdvisoryResponse, error) {
	work, err := llmEvidenceWorkForPortableJob(job)
	if err != nil {
		return llm.AdvisoryResponse{}, err
	}
	return s.llmGenerateResponseWithEvidenceTrace(
		ctx, stepID, "portable_advisory_worker", modelName, prompt, nil, work, true,
	)
}

func llmEvidenceWorkForPortableJob(job assemblyline.PortableJob) (llmEvidenceWork, error) {
	if err := job.Validate(); err != nil {
		return llmEvidenceWork{}, fmt.Errorf("record portable LLM work identity: %w", err)
	}
	return llmEvidenceWork{ID: job.ID, Kind: string(job.Kind)}, nil
}

func newLLMCallEvidenceRecord(
	stepID int64,
	scope, requestedModel, prompt string,
	responseSchema map[string]any,
	contract llmResponseContract,
	contextTokens, attempt int,
	work llmEvidenceWork,
) queue.LLMCallEvidenceRecord {
	format := contract.Format
	if format == "" {
		format = "text"
	}
	return queue.LLMCallEvidenceRecord{
		StepID: stepID, Scope: scope, WorkID: work.ID, WorkKind: work.Kind,
		RequestedModel: requestedModel, Model: requestedModel, Attempt: attempt,
		SystemPrompt: prompt, UserPrompt: contract.PromptHint,
		ResponseFormat: format, ResponseSchema: responseSchema,
		ContextTokens: contextTokens, MaxOutputTokens: contract.MaxTokens,
	}
}

func applyPreparedRequestToEvidence(record *queue.LLMCallEvidenceRecord, prepared llm.PreparedModel) {
	if record == nil {
		return
	}
	record.Model = effectivePreparedModel(prepared, record.RequestedModel)
	record.SystemPrompt = prepared.Prompt
	record.UserPrompt = prepared.PromptHint
	record.ContextTokens = prepared.ContextTokens
	record.MaxOutputTokens = prepared.MaxOutputTokens
	record.ResponseFormat = prepared.ResponseFormat
	if record.ResponseFormat == "" {
		record.ResponseFormat = "text"
	}
	record.ResponseSchema = prepared.ResponseSchema
	record.ThinkingEnabled = prepared.ThinkingEnabled
}

func effectivePreparedModel(prepared llm.PreparedModel, requested string) string {
	for _, candidate := range []string{prepared.ContextModel, prepared.BaseModel, requested} {
		if candidate = strings.TrimSpace(candidate); candidate != "" {
			return candidate
		}
	}
	return ""
}

func (s *Service) persistLLMCallEvidence(
	ctx context.Context,
	record queue.LLMCallEvidenceRecord,
	status queue.LLMCallEvidenceStatus,
	response string,
	callErr error,
	latency time.Duration,
) error {
	if s == nil || s.repo == nil {
		return llmEvidencePersistenceError{PersistenceError: fmt.Errorf("PostgreSQL repository is unavailable"), CallError: callErr}
	}
	record.Status = status
	record.Response = response
	if callErr != nil {
		record.Error = callErr.Error()
	}
	record.LatencyMS = latency.Milliseconds()
	if _, err := s.repo.RecordLLMCallEvidence(ctx, record); err != nil {
		return llmEvidencePersistenceError{PersistenceError: err, CallError: callErr}
	}
	return nil
}
