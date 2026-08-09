package worker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

func (s *Service) llmGenerateWithTrace(ctx context.Context, stepID int64, scope, modelName, prompt string) (string, error) {
	return s.llmGenerateWithSchemaTrace(ctx, stepID, scope, modelName, prompt, nil)
}

func (s *Service) llmGenerateWithSchemaTrace(
	ctx context.Context,
	stepID int64,
	scope, modelName, prompt string,
	responseSchema map[string]any,
) (string, error) {
	return s.llmGenerateWithEvidenceTrace(ctx, stepID, scope, modelName, prompt, responseSchema, llmEvidenceWork{})
}

func (s *Service) llmGenerateWithEvidenceTrace(
	ctx context.Context,
	stepID int64,
	scope, modelName, prompt string,
	responseSchema map[string]any,
	work llmEvidenceWork,
) (string, error) {
	return s.llmGenerateResponseWithEvidenceTrace(
		ctx, stepID, scope, modelName, prompt, responseSchema, work,
	)
}

func (s *Service) llmGenerateResponseWithEvidenceTrace(
	ctx context.Context,
	stepID int64,
	scope, modelName, prompt string,
	responseSchema map[string]any,
	work llmEvidenceWork,
) (string, error) {
	scope = safeLine(scope, "")
	if scope == "" {
		return "", fmt.Errorf("LLM scope is required")
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return "", fmt.Errorf("LLM model is required for scope %q", scope)
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "", fmt.Errorf("LLM prompt is required for scope %q", scope)
	}
	contract, err := llmResponseContractForScope(scope)
	if err != nil {
		return "", err
	}
	s.emitStepEvent(stepID, "llm_prompt", fmt.Sprintf("scope=%s model=%s chars=%d", scope, modelName, len(prompt)))
	s.emitStepContextWithBudget(stepID, "llm_prompt", strings.Join([]string{
		"scope=" + scope,
		"model=" + modelName,
		fmt.Sprintf("prompt_chars=%d", len(prompt)),
		prompt,
	}, "\n"), 14000)

	response, err := s.llmGenerateSingleAttempt(
		ctx, stepID, scope, modelName, prompt, responseSchema, contract, work, 1,
	)
	if err == nil {
		return response, nil
	}
	attemptErrors := []string{fmt.Sprintf("%s: %s", modelName, trimForBudget(err.Error(), 240))}
	if shouldRetrySameModelAfterCreateEOF(err) {
		s.emitStepEvent(stepID, "llm_retry_same_model", fmt.Sprintf("scope=%s model=%s reason=create_eof", scope, modelName))
		response, retryErr := s.llmGenerateSingleAttempt(
			ctx, stepID, scope, modelName, prompt, responseSchema, contract, work, 2,
		)
		if retryErr == nil {
			return response, nil
		}
		attemptErrors = append(attemptErrors, fmt.Sprintf("%s(retry): %s", modelName, trimForBudget(retryErr.Error(), 240)))
	}
	finalErr := fmt.Errorf("LLM generation failed after %d attempt(s) with configured model %q: %s", len(attemptErrors), modelName, strings.Join(attemptErrors, " | "))
	s.emitStepEvent(stepID, "llm_error", fmt.Sprintf("scope=%s model=%s", scope, modelName))
	s.emitStepContextWithBudget(stepID, "llm_error", strings.Join([]string{
		"scope=" + scope,
		"model=" + modelName,
		"error=" + trimForBudget(finalErr.Error(), 1400),
	}, "\n"), 3200)
	return "", finalErr
}

func (s *Service) llmGenerateSingleAttempt(
	ctx context.Context,
	stepID int64,
	scope, modelName, prompt string,
	responseSchema map[string]any,
	contract llmResponseContract,
	work llmEvidenceWork,
	attempt int,
) (string, error) {
	started := time.Now()
	evidence := newLLMCallEvidenceRecord(
		stepID, scope, modelName, prompt, responseSchema, contract, s.inferenceContextTokens, attempt, work,
	)
	stopHeartbeat := s.startProgressHeartbeat(ctx, stepID, fmt.Sprintf("llm:%s:attempt-%d", scope, attempt))
	defer stopHeartbeat()
	prepared, err := s.llm.PrepareContextModel(ctx, modelName, prompt)
	if err != nil {
		latency := time.Since(started)
		if evidenceErr := s.persistLLMCallEvidence(ctx, evidence, queue.LLMEvidencePreparationFailed, "", err, latency); evidenceErr != nil {
			return "", evidenceErr
		}
		s.recordWorkerLLMCall(ctx, stepID, scope, modelName, len(prompt), attempt, false, err, latency)
		return "", err
	}
	defer s.llm.CleanupPreparedModel(prepared)
	prepared.PromptHint = contract.PromptHint
	prepared.MaxOutputTokens = contract.MaxTokens
	prepared.ContextTokens = s.inferenceContextTokens
	prepared.ResponseFormat = contract.Format
	prepared.ResponseSchema = responseSchema
	prepared.ThinkingEnabled = false
	applyPreparedRequestToEvidence(&evidence, prepared)
	if err := llm.ValidateResponseContract(prepared); err != nil {
		callErr := fmt.Errorf("prepare response contract for scope %q: %w", scope, err)
		latency := time.Since(started)
		if evidenceErr := s.persistLLMCallEvidence(ctx, evidence, queue.LLMEvidencePreparationFailed, "", callErr, latency); evidenceErr != nil {
			return "", evidenceErr
		}
		s.recordWorkerLLMCall(ctx, stepID, scope, modelName, len(prompt), attempt, false, callErr, latency)
		return "", callErr
	}
	if err := llm.ValidateInferenceBudget(prepared.ContextTokens, prepared.MaxOutputTokens, prepared.Prompt, prepared.PromptHint); err != nil {
		callErr := fmt.Errorf("prepare inference budget for scope %q: %w", scope, err)
		latency := time.Since(started)
		if evidenceErr := s.persistLLMCallEvidence(ctx, evidence, queue.LLMEvidencePreparationFailed, "", callErr, latency); evidenceErr != nil {
			return "", evidenceErr
		}
		s.recordWorkerLLMCall(ctx, stepID, scope, modelName, len(prompt), attempt, false, callErr, latency)
		return "", callErr
	}
	s.emitStepEvent(stepID, "llm_model_prepared", fmt.Sprintf("scope=%s model=%s context_model=%s", scope, modelName, safeLine(prepared.ContextModel, "unknown")))
	s.emitStepContextWithBudget(stepID, "llm_model_prepare", strings.Join([]string{
		"scope=" + scope,
		"base_model=" + safeLine(prepared.BaseModel, modelName),
		"context_model=" + safeLine(prepared.ContextModel, "unknown"),
		"modelfile_path=" + safeLine(prepared.ModelfilePath, "unknown"),
		"prompt_hint=" + trimForBudget(prepared.PromptHint, 420),
		fmt.Sprintf("max_output_tokens=%d", prepared.MaxOutputTokens),
		fmt.Sprintf("context_tokens=%d", prepared.ContextTokens),
		"response_format=" + safeLine(prepared.ResponseFormat, "text"),
		fmt.Sprintf("response_schema=%t", len(prepared.ResponseSchema) > 0),
		fmt.Sprintf("thinking_enabled=%t", prepared.ThinkingEnabled),
	}, "\n"), 3200)

	response, err := s.generatePreparedWithProgress(ctx, stepID, scope, prepared)
	latency := time.Since(started)
	if err != nil {
		if evidenceErr := s.persistLLMCallEvidence(ctx, evidence, queue.LLMEvidenceGenerationFailed, response, err, latency); evidenceErr != nil {
			return "", evidenceErr
		}
		s.recordWorkerLLMCall(ctx, stepID, scope, modelName, len(prompt), attempt, false, err, latency)
		return "", err
	}
	if strings.TrimSpace(response) == "" {
		err = fmt.Errorf("LLM scope %q returned empty output", scope)
		if evidenceErr := s.persistLLMCallEvidence(ctx, evidence, queue.LLMEvidenceEmptyResponse, response, err, latency); evidenceErr != nil {
			return "", evidenceErr
		}
		s.recordWorkerLLMCall(ctx, stepID, scope, modelName, len(prompt), attempt, false, err, latency)
		return "", err
	}
	if evidenceErr := s.persistLLMCallEvidence(ctx, evidence, queue.LLMEvidenceSucceeded, response, nil, latency); evidenceErr != nil {
		return "", evidenceErr
	}
	s.recordWorkerLLMCall(ctx, stepID, scope, modelName, len(prompt), attempt, true, nil, latency)
	s.emitStepEvent(stepID, "llm_response", fmt.Sprintf(
		"scope=%s model=%s content_chars=%d",
		scope, modelName, len(response),
	))
	s.emitStepContextWithBudget(stepID, "llm_response", strings.Join([]string{
		"scope=" + scope,
		"model=" + modelName,
		fmt.Sprintf("response_chars=%d", len(response)),
		response,
	}, "\n"), 14000)
	return response, nil
}

func shouldRetrySameModelAfterCreateEOF(err error) bool {
	if err == nil {
		return false
	}
	var persistenceErr llmEvidencePersistenceError
	if errors.As(err, &persistenceErr) {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "ollama create failed") && strings.Contains(message, "eof")
}
