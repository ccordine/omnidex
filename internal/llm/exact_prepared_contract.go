package llm

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	PreparedGenerationSchemaV1            = "omnidex.prepared-generation.v1"
	MaxExactPreparedProviderResponseBytes = 16 * 1024 * 1024
)

var exactSHA256Digest = regexp.MustCompile(`^[0-9a-f]{64}$`)

type ProviderResponseDisposition string

const (
	ProviderResponseSucceeded      ProviderResponseDisposition = "succeeded"
	ProviderResponseTransportError ProviderResponseDisposition = "transport_error"
	ProviderResponseHTTPError      ProviderResponseDisposition = "http_error"
	ProviderResponseBodyLimit      ProviderResponseDisposition = "body_limit"
	ProviderResponseBodyReadError  ProviderResponseDisposition = "body_read_error"
	ProviderResponseInvalidJSON    ProviderResponseDisposition = "invalid_json"
	ProviderResponseEmptyContent   ProviderResponseDisposition = "empty_content"
)

type ProviderGenerationUsage struct {
	PromptEvalCount         int   `json:"prompt_eval_count"`
	EvalCount               int   `json:"eval_count"`
	TotalDurationNanos      int64 `json:"total_duration_nanos"`
	LoadDurationNanos       int64 `json:"load_duration_nanos"`
	PromptEvalDurationNanos int64 `json:"prompt_eval_duration_nanos"`
	EvalDurationNanos       int64 `json:"eval_duration_nanos"`
}

type PreparedGeneration struct {
	Schema                        string                          `json:"schema"`
	Protocol                      ExactPreparedProtocol           `json:"protocol"`
	ProviderRequestDisposition    ProviderRequestDisposition      `json:"provider_request_disposition"`
	Content                       string                          `json:"content"`
	ProviderRequestSHA256         string                          `json:"provider_request_sha256"`
	ProviderHTTPStatus            int                             `json:"provider_http_status"`
	ProviderResponseDisposition   ProviderResponseDisposition     `json:"provider_response_disposition"`
	ProviderResponseComplete      bool                            `json:"provider_response_complete"`
	ProviderContentEncoding       ProviderContentEncodingEvidence `json:"provider_content_encoding"`
	ProviderResponseBytesKnown    bool                            `json:"provider_response_bytes_known"`
	ProviderResponseSHA256        string                          `json:"provider_response_sha256"`
	ProviderResponseBytes         int64                           `json:"provider_response_bytes"`
	ProviderResponseCaptureSHA256 string                          `json:"provider_response_capture_sha256"`
	ProviderResponseCapturedBytes int                             `json:"provider_response_captured_bytes"`
	ProviderResponseCapture       []byte                          `json:"-"`
	ProviderDonePresent           bool                            `json:"provider_done_present"`
	ProviderDone                  bool                            `json:"provider_done"`
	ProviderDoneReason            string                          `json:"provider_done_reason"`
	UsagePresent                  bool                            `json:"usage_present"`
	Usage                         ProviderGenerationUsage         `json:"usage"`
}

func (usage ProviderGenerationUsage) ValidateSuccessful() error {
	if err := usage.Validate(); err != nil {
		return err
	}
	if usage.PromptEvalCount <= 0 || usage.EvalCount <= 0 ||
		usage.TotalDurationNanos <= 0 || usage.PromptEvalDurationNanos <= 0 ||
		usage.EvalDurationNanos <= 0 {
		return fmt.Errorf("successful exact generation requires positive native counts and durations")
	}
	remaining := usage.TotalDurationNanos
	for _, component := range []int64{
		usage.LoadDurationNanos,
		usage.PromptEvalDurationNanos,
		usage.EvalDurationNanos,
	} {
		if component > remaining {
			return fmt.Errorf("exact generation total duration is smaller than its native components")
		}
		remaining -= component
	}
	return nil
}

func (usage ProviderGenerationUsage) Validate() error {
	if usage.PromptEvalCount < 0 || usage.EvalCount < 0 ||
		usage.TotalDurationNanos < 0 || usage.LoadDurationNanos < 0 ||
		usage.PromptEvalDurationNanos < 0 || usage.EvalDurationNanos < 0 {
		return fmt.Errorf("exact prepared provider usage cannot be negative")
	}
	return nil
}

func (generation PreparedGeneration) Validate() error {
	if err := generation.validateSuccessfulContentEvidence(); err != nil {
		return err
	}
	if generation.ProviderDoneReason != "stop" {
		return fmt.Errorf("exact prepared generation content is invalid")
	}
	return nil
}

// validateSuccessfulContentEvidence validates a complete, successful provider
// response without treating its registered stop disposition as semantic
// completion. Request-bound validation classifies an exact `length` receipt
// only after it has also proven the response belongs to the frozen request.
func (generation PreparedGeneration) validateSuccessfulContentEvidence() error {
	if err := generation.ValidateInvocationEvidence(); err != nil {
		return err
	}
	if generation.ProviderResponseDisposition != ProviderResponseSucceeded ||
		strings.TrimSpace(generation.Content) == "" ||
		!utf8.ValidString(generation.Content) || strings.ContainsRune(generation.Content, 0) ||
		!generation.ProviderDonePresent || !generation.ProviderDone ||
		(generation.ProviderDoneReason != "stop" && generation.ProviderDoneReason != "length") ||
		!generation.UsagePresent {
		return fmt.Errorf("exact prepared generation content is invalid")
	}
	if err := generation.Usage.ValidateSuccessful(); err != nil {
		return err
	}
	return nil
}

func (generation PreparedGeneration) ValidateInvocationEvidence() error {
	return generation.ValidateProviderResponseEvidence()
}

// ValidateProviderResponseEvidence validates the exact dispatch and raw
// response receipt returned by the provider adapter.
func (generation PreparedGeneration) ValidateProviderResponseEvidence() error {
	if err := generation.ValidateProviderResponseReceipt(); err != nil {
		return err
	}
	if generation.ProviderResponseDisposition == ProviderResponseTransportError {
		if len(generation.ProviderResponseCapture) != 0 {
			return fmt.Errorf("transport failure contains provider response bytes")
		}
		return nil
	}
	if len(generation.ProviderResponseCapture) != generation.ProviderResponseCapturedBytes ||
		providerBodySHA256(generation.ProviderResponseCapture) !=
			generation.ProviderResponseCaptureSHA256 {
		return fmt.Errorf("provider response capture bytes differ from their receipt")
	}
	return ValidateExactPreparedResponseProjection(generation)
}

// ValidateProviderResponseReceipt validates the normalized provider receipt
// without claiming that its out-of-line raw response bytes were supplied.
func (generation PreparedGeneration) ValidateProviderResponseReceipt() error {
	if generation.Schema != PreparedGenerationSchemaV1 || generation.Protocol.Validate() != nil ||
		generation.ProviderRequestDisposition.Validate() != nil ||
		!exactSHA256Digest.MatchString(generation.ProviderRequestSHA256) {
		return fmt.Errorf("exact prepared provider response evidence is invalid")
	}
	if generation.ProviderResponseDisposition == ProviderResponseTransportError {
		if generation.ProviderHTTPStatus != 0 || generation.ProviderResponseComplete ||
			generation.ProviderResponseBytesKnown ||
			generation.ProviderContentEncoding != (ProviderContentEncodingEvidence{}) ||
			generation.ProviderResponseSHA256 != "" || generation.ProviderResponseBytes != 0 ||
			generation.ProviderResponseCaptureSHA256 != "" ||
			generation.ProviderResponseCapturedBytes != 0 ||
			generation.ProviderDonePresent || generation.ProviderDone || generation.ProviderDoneReason != "" {
			return fmt.Errorf("transport failure claims a provider response")
		}
		return nil
	}
	if generation.ProviderRequestDisposition != ProviderRequestDispatched {
		return fmt.Errorf("provider response claims a request that was not fully dispatched")
	}
	if !registeredProviderResponseDisposition(generation.ProviderResponseDisposition) ||
		generation.ProviderHTTPStatus < 100 || generation.ProviderHTTPStatus > 599 ||
		generation.ProviderResponseBytes < 0 || generation.ProviderResponseCapturedBytes < 0 ||
		generation.ProviderResponseCapturedBytes > MaxExactPreparedProviderResponseBytes+1 ||
		!exactSHA256Digest.MatchString(generation.ProviderResponseCaptureSHA256) ||
		generation.ProviderContentEncoding.Validate() != nil {
		return fmt.Errorf("provider response receipt is invalid")
	}
	if generation.ProviderResponseComplete {
		if generation.ProviderResponseDisposition == ProviderResponseBodyLimit ||
			generation.ProviderResponseDisposition == ProviderResponseBodyReadError {
			return fmt.Errorf("partial provider response disposition claims a complete body")
		}
		if !generation.ProviderResponseBytesKnown ||
			!exactSHA256Digest.MatchString(generation.ProviderResponseSHA256) ||
			generation.ProviderResponseCapturedBytes > MaxExactPreparedProviderResponseBytes ||
			generation.ProviderResponseBytes != int64(generation.ProviderResponseCapturedBytes) ||
			generation.ProviderResponseSHA256 != generation.ProviderResponseCaptureSHA256 {
			return fmt.Errorf("complete provider response lacks its raw body identity")
		}
	} else {
		if generation.ProviderResponseBytesKnown || generation.ProviderResponseBytes != 0 ||
			generation.ProviderResponseSHA256 != "" ||
			(generation.ProviderResponseDisposition != ProviderResponseBodyLimit &&
				generation.ProviderResponseDisposition != ProviderResponseBodyReadError) {
			return fmt.Errorf("partial provider response claims a complete raw body identity")
		}
		if generation.ProviderResponseDisposition == ProviderResponseBodyLimit &&
			generation.ProviderResponseCapturedBytes != MaxExactPreparedProviderResponseBytes+1 {
			return fmt.Errorf("provider body-limit receipt lacks the exact bounded capture")
		}
	}
	parsedFinal := generation.ProviderResponseDisposition == ProviderResponseSucceeded ||
		generation.ProviderResponseDisposition == ProviderResponseEmptyContent
	if parsedFinal &&
		(generation.ProviderHTTPStatus < 200 || generation.ProviderHTTPStatus >= 300 ||
			!generation.ProviderContentEncoding.IsIdentity() ||
			!generation.ProviderResponseComplete ||
			(!generation.ProviderDonePresent && generation.ProviderDone) ||
			(generation.ProviderDoneReason != "" && generation.ProviderDoneReason != "stop" &&
				generation.ProviderDoneReason != "length") || generation.Usage.Validate() != nil) {
		return fmt.Errorf("successful provider response receipt is incomplete")
	}
	if !parsedFinal &&
		(generation.ProviderDonePresent ||
			generation.ProviderDone || generation.ProviderDoneReason != "" || generation.UsagePresent ||
			generation.Usage != (ProviderGenerationUsage{})) {
		return fmt.Errorf("unsuccessful provider response claims a completion reason")
	}
	return nil
}

func registeredProviderResponseDisposition(value ProviderResponseDisposition) bool {
	switch value {
	case ProviderResponseSucceeded, ProviderResponseHTTPError, ProviderResponseBodyLimit,
		ProviderResponseBodyReadError, ProviderResponseInvalidJSON, ProviderResponseEmptyContent:
		return true
	default:
		return false
	}
}

// ExactPreparedContractClient is an explicit provider capability. Cognition
// policy must reject providers that cannot enforce every PreparedModel field.
type ExactPreparedContractClient interface {
	RequireExactPreparedContract() error
	ValidateExactPreparedContract(PreparedModel) error
	GeneratePreparedExact(context.Context, PreparedModel) (PreparedGeneration, error)
}

func RequireExactPreparedContract(client ExactPreparedContractClient) (ExactPreparedContractClient, error) {
	if client == nil {
		return nil, fmt.Errorf("configured generation provider does not enforce the exact prepared contract")
	}
	if err := client.RequireExactPreparedContract(); err != nil {
		return nil, err
	}
	return client, nil
}
