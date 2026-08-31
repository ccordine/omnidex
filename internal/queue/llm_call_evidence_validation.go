package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
)

const maxLLMCallGenerationReceiptBytes = llm.MaxOwnedPreparedGenerationBytes + (1024 * 1024)

type normalizedLLMCallOpening struct {
	record                LLMCallOpeningRecord
	modelInput            string
	modelInputSHA256      string
	providerRequest       []byte
	providerRequestSHA256 string
}

type normalizedLLMCallReceipt struct {
	record                  LLMCallReceiptRecord
	modelInput              string
	modelInputSHA256        string
	providerRequest         []byte
	providerRequestSHA256   string
	generationReceipt       []byte
	generationReceiptSHA256 string
	rawResponsePresent      bool
	rawResponse             []byte
	rawResponseSHA256       string
	candidateSHA256         string
	status                  LLMCallStatus
}

func normalizeLLMCallOpening(record LLMCallOpeningRecord) (normalizedLLMCallOpening, error) {
	if err := validateStepAttemptAuthority(record.Authority); err != nil {
		return normalizedLLMCallOpening{}, err
	}
	wantedScope, err := assemblyline.PortableWorkerScopeForWorkKind(record.WorkKind)
	if err != nil {
		return normalizedLLMCallOpening{}, err
	}
	if record.Scope != wantedScope || record.Scope != strings.TrimSpace(record.Scope) {
		return normalizedLLMCallOpening{}, fmt.Errorf("LLM call evidence scope does not match its work kind")
	}
	if !exactLowerSHA256(record.WorkID) {
		return normalizedLLMCallOpening{}, fmt.Errorf("LLM call evidence work ID must be one exact SHA-256")
	}
	if record.RequestedModel == "" || record.RequestedModel != strings.TrimSpace(record.RequestedModel) ||
		record.Prepared.BaseModel != record.RequestedModel ||
		record.Prepared.ContextModel == "" {
		return normalizedLLMCallOpening{}, fmt.Errorf("LLM call evidence model authority is incomplete")
	}

	modelInput, err := llm.ExactPreparedRequestModelInput(record.Prepared)
	if err != nil {
		return normalizedLLMCallOpening{}, fmt.Errorf("render exact model input evidence: %w", err)
	}
	providerRequest, err := llm.ExactPreparedRequestBytes(record.Prepared)
	if err != nil {
		return normalizedLLMCallOpening{}, fmt.Errorf("render exact provider request evidence: %w", err)
	}
	return normalizedLLMCallOpening{
		record: record, modelInput: modelInput,
		modelInputSHA256:      llmEvidenceSHA256([]byte(modelInput)),
		providerRequest:       append([]byte(nil), providerRequest...),
		providerRequestSHA256: llmEvidenceSHA256(providerRequest),
	}, nil
}

func normalizeLLMCallReceipt(record LLMCallReceiptRecord) (normalizedLLMCallReceipt, error) {
	if err := validateStepAttemptAuthority(record.Authority); err != nil {
		return normalizedLLMCallReceipt{}, err
	}
	if record.CallEvidenceID < 1 || record.Elapsed < 0 {
		return normalizedLLMCallReceipt{}, fmt.Errorf("LLM call receipt identity or elapsed duration is invalid")
	}
	if err := validateLLMCallError(record.CallError); err != nil {
		return normalizedLLMCallReceipt{}, err
	}
	modelInput, err := llm.ExactPreparedRequestModelInput(record.Prepared)
	if err != nil {
		return normalizedLLMCallReceipt{}, fmt.Errorf("render exact receipt model input: %w", err)
	}
	providerRequest, err := llm.ExactPreparedRequestBytes(record.Prepared)
	if err != nil {
		return normalizedLLMCallReceipt{}, fmt.Errorf("render exact receipt provider request: %w", err)
	}
	owned, err := llm.OwnBoundedPreparedGeneration(record.Generation)
	if err != nil {
		return normalizedLLMCallReceipt{}, fmt.Errorf("own exact provider response evidence: %w", err)
	}
	record.Generation = owned
	if record.CallError == "" {
		if err := llm.ValidateExactPreparedGenerationForRequest(record.Prepared, owned); err != nil {
			return normalizedLLMCallReceipt{}, fmt.Errorf("successful LLM evidence has invalid provider result: %w", err)
		}
	}
	generationReceipt, err := json.Marshal(owned)
	if err != nil {
		return normalizedLLMCallReceipt{}, fmt.Errorf("encode exact provider generation receipt: %w", err)
	}
	if len(generationReceipt) > maxLLMCallGenerationReceiptBytes {
		return normalizedLLMCallReceipt{}, fmt.Errorf("exact provider generation receipt exceeds its evidence bound")
	}
	rawPresent := llmGenerationHasRawResponse(owned)
	rawResponse := []byte(nil)
	rawResponseSHA256 := ""
	if rawPresent {
		rawResponse = make([]byte, len(owned.ProviderResponseCapture))
		copy(rawResponse, owned.ProviderResponseCapture)
		rawResponseSHA256 = llmEvidenceSHA256(rawResponse)
	}
	status := LLMCallSucceeded
	if record.CallError != "" {
		status = LLMCallFailed
	}
	candidateSHA256 := ""
	if owned.Content != "" {
		candidateSHA256 = llmEvidenceSHA256([]byte(owned.Content))
	}
	return normalizedLLMCallReceipt{
		record: record, modelInput: modelInput,
		modelInputSHA256:        llmEvidenceSHA256([]byte(modelInput)),
		providerRequest:         append([]byte(nil), providerRequest...),
		providerRequestSHA256:   llmEvidenceSHA256(providerRequest),
		generationReceipt:       generationReceipt,
		generationReceiptSHA256: llmEvidenceSHA256(generationReceipt),
		rawResponsePresent:      rawPresent,
		rawResponse:             rawResponse,
		rawResponseSHA256:       rawResponseSHA256,
		candidateSHA256:         candidateSHA256,
		status:                  status,
	}, nil
}

func llmGenerationHasRawResponse(generation llm.PreparedGeneration) bool {
	return generation.ProviderHTTPStatus != 0 ||
		generation.ProviderResponseDisposition != "" &&
			generation.ProviderResponseDisposition != llm.ProviderResponseTransportError ||
		generation.ProviderResponseCaptureSHA256 != "" ||
		generation.ProviderResponseCapturedBytes != 0 ||
		len(generation.ProviderResponseCapture) != 0
}

func validateLLMCallError(value string) error {
	if value == "" {
		return nil
	}
	if value != strings.TrimSpace(value) || len(value) > 8192 || !utf8.ValidString(value) ||
		strings.ContainsRune(value, 0) {
		return fmt.Errorf("LLM call evidence error must be exact bounded UTF-8 text")
	}
	return nil
}

func exactLowerSHA256(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func llmEvidenceSHA256(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
