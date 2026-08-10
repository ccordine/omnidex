package llm

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	PreparedGenerationSchemaV1            = "omnidex.prepared-generation.v1"
	MaxExactPreparedProviderResponseBytes = 16 * 1024 * 1024
)

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
	Schema string `json:"schema"`
	// ProviderRequestDispatched means the exact provider request was dispatched. A
	// transport failure after dispatch is true; any pre-dispatch failure is false.
	ProviderRequestDispatched     bool                        `json:"provider_request_dispatched"`
	Content                       string                      `json:"content"`
	ProviderRequestSHA256         string                      `json:"provider_request_sha256"`
	ProviderHTTPStatus            int                         `json:"provider_http_status"`
	ProviderResponseDisposition   ProviderResponseDisposition `json:"provider_response_disposition"`
	ProviderResponseComplete      bool                        `json:"provider_response_complete"`
	ProviderContentEncodingCount  int                         `json:"provider_content_encoding_count"`
	ProviderContentEncoding       string                      `json:"provider_content_encoding"`
	ProviderResponseUncompressed  bool                        `json:"provider_response_uncompressed"`
	ProviderResponseBytesKnown    bool                        `json:"provider_response_bytes_known"`
	ProviderResponseSHA256        string                      `json:"provider_response_sha256"`
	ProviderResponseBytes         int64                       `json:"provider_response_bytes"`
	ProviderResponseCaptureSHA256 string                      `json:"provider_response_capture_sha256"`
	ProviderResponseCapturedBytes int                         `json:"provider_response_captured_bytes"`
	ProviderResponseCapture       []byte                      `json:"-"`
	ProviderResponseModel         string                      `json:"provider_response_model"`
	ProviderDonePresent           bool                        `json:"provider_done_present"`
	ProviderDone                  bool                        `json:"provider_done"`
	ProviderDoneReason            string                      `json:"provider_done_reason"`
	UsagePresent                  bool                        `json:"usage_present"`
	Usage                         ProviderGenerationUsage     `json:"usage"`
	ProviderObservation           ProviderIdentityObservation `json:"provider_observation"`
	// ProviderIdentityEvidence is persisted out of line. The normalized
	// observation above binds its content-addressed reference; keeping the raw
	// bodies out of the prepared-generation JSON prevents large provider
	// identity responses from entering prompts or terminal trace payloads.
	ProviderIdentityEvidence ProviderIdentityEvidence `json:"-"`
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
	return generation.ProviderObservation.Validate()
}

func (generation PreparedGeneration) ValidateInvocationEvidence() error {
	if err := generation.ProviderObservation.Validate(); err != nil {
		return err
	}
	if err := generation.ProviderObservation.ValidateEvidence(
		generation.ProviderIdentityEvidence,
	); err != nil {
		return err
	}
	return generation.ValidateProviderResponseEvidence()
}

// ValidateProviderResponseEvidence validates only the exact dispatch and raw
// response receipt. It intentionally does not trust or validate the separately
// challenge-bound provider identity observation.
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
	if generation.Schema != PreparedGenerationSchemaV1 || !generation.ProviderRequestDispatched ||
		!providerIdentityDigest.MatchString(generation.ProviderRequestSHA256) {
		return fmt.Errorf("exact prepared provider response evidence is invalid")
	}
	if generation.ProviderResponseDisposition == ProviderResponseTransportError {
		if generation.ProviderHTTPStatus != 0 || generation.ProviderResponseComplete ||
			generation.ProviderResponseBytesKnown ||
			generation.ProviderContentEncodingCount != 0 ||
			generation.ProviderContentEncoding != "" || generation.ProviderResponseUncompressed ||
			generation.ProviderResponseSHA256 != "" || generation.ProviderResponseBytes != 0 ||
			generation.ProviderResponseCaptureSHA256 != "" ||
			generation.ProviderResponseCapturedBytes != 0 || generation.ProviderResponseModel != "" ||
			generation.ProviderDonePresent || generation.ProviderDone || generation.ProviderDoneReason != "" {
			return fmt.Errorf("transport failure claims a provider response")
		}
		return nil
	}
	if !registeredProviderResponseDisposition(generation.ProviderResponseDisposition) ||
		generation.ProviderHTTPStatus < 100 || generation.ProviderHTTPStatus > 599 ||
		generation.ProviderResponseBytes < 0 || generation.ProviderResponseCapturedBytes < 0 ||
		generation.ProviderResponseCapturedBytes > MaxExactPreparedProviderResponseBytes+1 ||
		!providerIdentityDigest.MatchString(generation.ProviderResponseCaptureSHA256) ||
		!validProviderContentEncoding(
			generation.ProviderContentEncodingCount, generation.ProviderContentEncoding,
		) {
		return fmt.Errorf("provider response receipt is invalid")
	}
	if generation.ProviderResponseComplete {
		if generation.ProviderResponseDisposition == ProviderResponseBodyLimit ||
			generation.ProviderResponseDisposition == ProviderResponseBodyReadError {
			return fmt.Errorf("partial provider response disposition claims a complete body")
		}
		if !generation.ProviderResponseBytesKnown ||
			!providerIdentityDigest.MatchString(generation.ProviderResponseSHA256) ||
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
			generation.ProviderResponseUncompressed || !ProviderContentEncodingIsIdentity(
				generation.ProviderContentEncodingCount, generation.ProviderContentEncoding,
			) ||
			!generation.ProviderResponseComplete || !validPreparedResponseModel(generation.ProviderResponseModel) ||
			(!generation.ProviderDonePresent && generation.ProviderDone) ||
			(generation.ProviderDoneReason != "" && generation.ProviderDoneReason != "stop" &&
				generation.ProviderDoneReason != "length") || generation.Usage.Validate() != nil) {
		return fmt.Errorf("successful provider response receipt is incomplete")
	}
	if !parsedFinal &&
		(generation.ProviderResponseModel != "" || generation.ProviderDonePresent ||
			generation.ProviderDone || generation.ProviderDoneReason != "" || generation.UsagePresent ||
			generation.Usage != (ProviderGenerationUsage{})) {
		return fmt.Errorf("unsuccessful provider response claims a completion reason")
	}
	return nil
}

func validPreparedResponseModel(model string) bool {
	return strings.TrimSpace(model) == model && model != "" && len(model) <= 512 &&
		utf8.ValidString(model) && !strings.ContainsRune(model, 0)
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
	ValidateExactPreparedProvider(ProviderIdentityExpectation) error
	ValidateExactPreparedContract(PreparedModel) error
	GeneratePreparedExact(context.Context, PreparedModel) (PreparedGeneration, error)
}

func ValidateExactPreparedProvider(client Client, expected ProviderIdentityExpectation) error {
	exact, err := RequireExactPreparedContract(client)
	if err != nil {
		return err
	}
	if err := exact.ValidateExactPreparedProvider(expected); err != nil {
		return fmt.Errorf("configured provider lacks the registered exact raw contract: %w", err)
	}
	return nil
}

func RequireExactPreparedContract(client Client) (ExactPreparedContractClient, error) {
	exact, ok := client.(ExactPreparedContractClient)
	if !ok {
		return nil, fmt.Errorf("configured generation provider does not enforce the exact prepared contract")
	}
	if err := exact.RequireExactPreparedContract(); err != nil {
		return nil, err
	}
	return exact, nil
}
