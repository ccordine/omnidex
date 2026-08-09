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
	response, err := s.llmGenerateResponseWithEvidenceTrace(
		ctx, stepID, scope, modelName, prompt, responseSchema, work, false,
	)
	return response.Content, err
}

func (s *Service) llmGenerateResponseWithEvidenceTrace(
	ctx context.Context,
	stepID int64,
	scope, modelName, prompt string,
	responseSchema map[string]any,
	work llmEvidenceWork,
	thinkingEnabled bool,
) (llm.AdvisoryResponse, error) {
	scope = safeLine(scope, "")
	if scope == "" {
		return llm.AdvisoryResponse{}, fmt.Errorf("LLM scope is required")
	}
	if thinkingEnabled && scope != "portable_advisory_worker" {
		return llm.AdvisoryResponse{}, fmt.Errorf("native advisory generation requires the registered portable advisory scope")
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return llm.AdvisoryResponse{}, fmt.Errorf("LLM model is required for scope %q", scope)
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return llm.AdvisoryResponse{}, fmt.Errorf("LLM prompt is required for scope %q", scope)
	}
	contract, err := llmResponseContractForScope(scope)
	if err != nil {
		return llm.AdvisoryResponse{}, err
	}
	s.emitStepEvent(stepID, "llm_prompt", fmt.Sprintf("scope=%s model=%s chars=%d", scope, modelName, len(prompt)))
	s.emitStepContextWithBudget(stepID, "llm_prompt", strings.Join([]string{
		"scope=" + scope,
		"model=" + modelName,
		fmt.Sprintf("prompt_chars=%d", len(prompt)),
		prompt,
	}, "\n"), 14000)

	response, err := s.llmGenerateSingleAttempt(
		ctx, stepID, scope, modelName, prompt, responseSchema, contract, work, thinkingEnabled, 1,
	)
	if err == nil {
		return response, nil
	}
	attemptErrors := []string{fmt.Sprintf("%s: %s", modelName, trimForBudget(err.Error(), 240))}
	if shouldRetrySameModelAfterCreateEOF(err) {
		s.emitStepEvent(stepID, "llm_retry_same_model", fmt.Sprintf("scope=%s model=%s reason=create_eof", scope, modelName))
		response, retryErr := s.llmGenerateSingleAttempt(
			ctx, stepID, scope, modelName, prompt, responseSchema, contract, work, thinkingEnabled, 2,
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
	return llm.AdvisoryResponse{}, finalErr
}

func (s *Service) llmGenerateSingleAttempt(
	ctx context.Context,
	stepID int64,
	scope, modelName, prompt string,
	responseSchema map[string]any,
	contract llmResponseContract,
	work llmEvidenceWork,
	thinkingEnabled bool,
	attempt int,
) (llm.AdvisoryResponse, error) {
	started := time.Now()
	evidence := newLLMCallEvidenceRecord(
		stepID, scope, modelName, prompt, responseSchema, contract, s.inferenceContextTokens, attempt, work,
	)
	evidence.ThinkingEnabled = thinkingEnabled
	stopHeartbeat := s.startProgressHeartbeat(ctx, stepID, fmt.Sprintf("llm:%s:attempt-%d", scope, attempt))
	defer stopHeartbeat()
	prepared, err := s.llm.PrepareContextModel(ctx, modelName, prompt)
	if err != nil {
		latency := time.Since(started)
		if evidenceErr := s.persistLLMCallEvidence(ctx, evidence, queue.LLMEvidencePreparationFailed, "", err, latency); evidenceErr != nil {
			return llm.AdvisoryResponse{}, evidenceErr
		}
		s.recordWorkerLLMCall(ctx, stepID, scope, modelName, len(prompt), attempt, false, err, latency)
		return llm.AdvisoryResponse{}, err
	}
	defer s.llm.CleanupPreparedModel(prepared)
	prepared.PromptHint = contract.PromptHint
	prepared.MaxOutputTokens = contract.MaxTokens
	prepared.ContextTokens = s.inferenceContextTokens
	prepared.ResponseFormat = contract.Format
	prepared.ResponseSchema = responseSchema
	prepared.ThinkingEnabled = thinkingEnabled
	applyPreparedRequestToEvidence(&evidence, prepared)
	if err := llm.ValidateResponseContract(prepared); err != nil {
		callErr := fmt.Errorf("prepare response contract for scope %q: %w", scope, err)
		latency := time.Since(started)
		if evidenceErr := s.persistLLMCallEvidence(ctx, evidence, queue.LLMEvidencePreparationFailed, "", callErr, latency); evidenceErr != nil {
			return llm.AdvisoryResponse{}, evidenceErr
		}
		s.recordWorkerLLMCall(ctx, stepID, scope, modelName, len(prompt), attempt, false, callErr, latency)
		return llm.AdvisoryResponse{}, callErr
	}
	if err := llm.ValidateInferenceBudget(prepared.ContextTokens, prepared.MaxOutputTokens, prepared.Prompt, prepared.PromptHint); err != nil {
		callErr := fmt.Errorf("prepare inference budget for scope %q: %w", scope, err)
		latency := time.Since(started)
		if evidenceErr := s.persistLLMCallEvidence(ctx, evidence, queue.LLMEvidencePreparationFailed, "", callErr, latency); evidenceErr != nil {
			return llm.AdvisoryResponse{}, evidenceErr
		}
		s.recordWorkerLLMCall(ctx, stepID, scope, modelName, len(prompt), attempt, false, callErr, latency)
		return llm.AdvisoryResponse{}, callErr
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

	response, err := s.generatePreparedResponseWithProgress(ctx, stepID, scope, prepared)
	latency := time.Since(started)
	evidenceResponse, evidenceResponseErr := llmResponseEvidence(response, thinkingEnabled)
	if evidenceResponseErr != nil {
		return llm.AdvisoryResponse{}, evidenceResponseErr
	}
	if err != nil {
		if evidenceErr := s.persistLLMCallEvidence(ctx, evidence, queue.LLMEvidenceGenerationFailed, evidenceResponse, err, latency); evidenceErr != nil {
			return llm.AdvisoryResponse{}, evidenceErr
		}
		s.recordWorkerLLMCall(ctx, stepID, scope, modelName, len(prompt), attempt, false, err, latency)
		return llm.AdvisoryResponse{}, err
	}
	if err := response.Validate(); err != nil {
		err = fmt.Errorf("LLM scope %q returned empty output", scope)
		if evidenceErr := s.persistLLMCallEvidence(ctx, evidence, queue.LLMEvidenceEmptyResponse, evidenceResponse, err, latency); evidenceErr != nil {
			return llm.AdvisoryResponse{}, evidenceErr
		}
		s.recordWorkerLLMCall(ctx, stepID, scope, modelName, len(prompt), attempt, false, err, latency)
		return llm.AdvisoryResponse{}, err
	}
	if evidenceErr := s.persistLLMCallEvidence(ctx, evidence, queue.LLMEvidenceSucceeded, evidenceResponse, nil, latency); evidenceErr != nil {
		return llm.AdvisoryResponse{}, evidenceErr
	}
	s.recordWorkerLLMCall(ctx, stepID, scope, modelName, len(prompt), attempt, true, nil, latency)
	s.emitStepEvent(stepID, "llm_response", fmt.Sprintf(
		"scope=%s model=%s content_chars=%d thinking_chars=%d",
		scope, modelName, len(response.Content), len(response.Thinking),
	))
	s.emitStepContextWithBudget(stepID, "llm_response", strings.Join([]string{
		"scope=" + scope,
		"model=" + modelName,
		fmt.Sprintf("response_chars=%d", len(response.Content)),
		response.Content,
	}, "\n"), 14000)
	return response, nil
}

func llmResponseEvidence(response llm.AdvisoryResponse, thinkingEnabled bool) (string, error) {
	if !thinkingEnabled {
		return response.Content, nil
	}
	if err := response.Validate(); err != nil {
		return "", nil
	}
	raw, err := response.EvidenceJSON()
	if err != nil {
		return "", fmt.Errorf("encode exact advisory evidence: %w", err)
	}
	return raw, nil
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
