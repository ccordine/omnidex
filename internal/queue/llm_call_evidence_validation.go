package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
)

const maxLLMCallGenerationReceiptBytes = 16 * 1024

// llmCallGenerationReceipt retains the provider-derived metadata and exact
// identities without serializing model content a second time. The candidate
// and raw response bytes are stored in their dedicated immutable columns.
type llmCallGenerationReceipt struct {
	Schema                        string                              `json:"schema"`
	Protocol                      llm.ExactPreparedProtocol           `json:"protocol"`
	ProviderRequestDisposition    llm.ProviderRequestDisposition      `json:"provider_request_disposition"`
	ContentBytes                  int                                 `json:"content_bytes"`
	ContentSHA256                 string                              `json:"content_sha256"`
	ProviderRequestSHA256         string                              `json:"provider_request_sha256"`
	ProviderHTTPStatus            int                                 `json:"provider_http_status"`
	ProviderResponseDisposition   llm.ProviderResponseDisposition     `json:"provider_response_disposition"`
	ProviderResponseComplete      bool                                `json:"provider_response_complete"`
	ProviderContentEncoding       llm.ProviderContentEncodingEvidence `json:"provider_content_encoding"`
	ProviderResponseBytesKnown    bool                                `json:"provider_response_bytes_known"`
	ProviderResponseSHA256        string                              `json:"provider_response_sha256"`
	ProviderResponseBytes         int64                               `json:"provider_response_bytes"`
	ProviderResponseCaptureSHA256 string                              `json:"provider_response_capture_sha256"`
	ProviderResponseCapturedBytes int                                 `json:"provider_response_captured_bytes"`
	ProviderDonePresent           bool                                `json:"provider_done_present"`
	ProviderDone                  bool                                `json:"provider_done"`
	ProviderDoneReason            string                              `json:"provider_done_reason"`
	UsagePresent                  bool                                `json:"usage_present"`
	Usage                         llm.ProviderGenerationUsage         `json:"usage"`
}

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
	if record.Iteration < 1 || record.Iteration > assemblyline.MaxSourceBodyAttempts {
		return normalizedLLMCallOpening{}, fmt.Errorf(
			"LLM call evidence iteration must be between 1 and %d",
			assemblyline.MaxSourceBodyAttempts,
		)
	}
	if record.OutputContinuation != 0 {
		return normalizedLLMCallOpening{}, fmt.Errorf(
			"LLM call evidence cannot authorize output continuation",
		)
	}
	if record.DispatchAttempt != 1 {
		return normalizedLLMCallOpening{}, fmt.Errorf(
			"LLM call evidence requires exactly one provider dispatch",
		)
	}
	if record.ReplacesCallEvidenceID != 0 {
		return normalizedLLMCallOpening{}, fmt.Errorf(
			"LLM call evidence cannot replace another provider dispatch",
		)
	}
	isLineageRoot := record.Iteration == 1
	if isLineageRoot && record.ParentCallEvidenceID != 0 {
		return normalizedLLMCallOpening{}, fmt.Errorf(
			"initial LLM call evidence cannot name a parent call",
		)
	}
	if !isLineageRoot && record.ParentCallEvidenceID < 1 {
		return normalizedLLMCallOpening{}, fmt.Errorf(
			"continued LLM call evidence requires one parent call",
		)
	}
	if record.Iteration > 1 && record.WorkKind != assemblyline.WorkFragmentGeneration {
		return normalizedLLMCallOpening{}, fmt.Errorf(
			"iterative LLM call evidence requires fragment generation",
		)
	}
	if record.Iteration == 1 && record.SourceCorrection != nil {
		return normalizedLLMCallOpening{}, fmt.Errorf(
			"initial LLM call evidence cannot carry source correction state",
		)
	}
	if record.Iteration > 1 && record.SourceCorrection == nil {
		return normalizedLLMCallOpening{}, fmt.Errorf(
			"iterative fragment evidence requires exact source correction state",
		)
	}
	if record.SourceCorrection != nil {
		if err := record.SourceCorrection.Validate(record.Prepared.Prompt); err != nil {
			return normalizedLLMCallOpening{}, fmt.Errorf(
				"LLM call source correction evidence: %w", err,
			)
		}
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
	generationErr := llm.ValidateExactPreparedGenerationForRequest(record.Prepared, owned)
	var outputLimit *llm.ExactPreparedOutputLimitReachedError
	validatedOutputLimit := errors.As(generationErr, &outputLimit)
	if record.OutputLimitReached != validatedOutputLimit {
		return normalizedLLMCallReceipt{}, fmt.Errorf(
			"LLM call output-limit classification differs from exact provider evidence",
		)
	}
	if record.CallError == "" {
		if generationErr != nil {
			return normalizedLLMCallReceipt{}, fmt.Errorf(
				"successful LLM evidence has invalid provider result: %w", generationErr,
			)
		}
	} else if record.OutputLimitReached && outputLimit.Validate() != nil {
		return normalizedLLMCallReceipt{}, fmt.Errorf(
			"LLM call output-limit evidence is invalid",
		)
	}
	generationReceipt, err := encodeLLMCallGenerationReceipt(owned)
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

func encodeLLMCallGenerationReceipt(
	generation llm.PreparedGeneration,
) ([]byte, error) {
	receipt := llmCallGenerationReceipt{
		Schema: generation.Schema, Protocol: generation.Protocol,
		ProviderRequestDisposition:    generation.ProviderRequestDisposition,
		ContentBytes:                  len(generation.Content),
		ContentSHA256:                 llmEvidenceSHA256([]byte(generation.Content)),
		ProviderRequestSHA256:         generation.ProviderRequestSHA256,
		ProviderHTTPStatus:            generation.ProviderHTTPStatus,
		ProviderResponseDisposition:   generation.ProviderResponseDisposition,
		ProviderResponseComplete:      generation.ProviderResponseComplete,
		ProviderContentEncoding:       generation.ProviderContentEncoding,
		ProviderResponseBytesKnown:    generation.ProviderResponseBytesKnown,
		ProviderResponseSHA256:        generation.ProviderResponseSHA256,
		ProviderResponseBytes:         generation.ProviderResponseBytes,
		ProviderResponseCaptureSHA256: generation.ProviderResponseCaptureSHA256,
		ProviderResponseCapturedBytes: generation.ProviderResponseCapturedBytes,
		ProviderDonePresent:           generation.ProviderDonePresent,
		ProviderDone:                  generation.ProviderDone,
		ProviderDoneReason:            generation.ProviderDoneReason,
		UsagePresent:                  generation.UsagePresent,
		Usage:                         generation.Usage,
	}
	return json.Marshal(receipt)
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
