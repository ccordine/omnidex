package llm

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/exactjson"
)

// ExactPreparedResponse is the sole normalized interpretation of a complete
// raw /api/generate response. The original bytes remain separate evidence.
type ExactPreparedResponse struct {
	Disposition  ProviderResponseDisposition
	Model        string
	Content      string
	DonePresent  bool
	Done         bool
	DoneReason   string
	UsagePresent bool
	Usage        ProviderGenerationUsage
}

type exactPreparedResponseWire struct {
	Model              string `json:"model"`
	CreatedAt          string `json:"created_at"`
	Response           string `json:"response"`
	Done               *bool  `json:"done,omitempty"`
	DoneReason         string `json:"done_reason,omitempty"`
	TotalDuration      *int64 `json:"total_duration,omitempty"`
	LoadDuration       *int64 `json:"load_duration,omitempty"`
	PromptEvalCount    *int   `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration *int64 `json:"prompt_eval_duration,omitempty"`
	EvalCount          *int   `json:"eval_count,omitempty"`
	EvalDuration       *int64 `json:"eval_duration,omitempty"`
}

// DecodeExactPreparedResponse derives every normalized response field from the
// exact captured bytes. It returns the derived partial receipt alongside any
// provider-contract failure so callers can journal the evidence durably.
func DecodeExactPreparedResponse(status int, body []byte) (ExactPreparedResponse, error) {
	if status < 100 || status > 599 {
		return ExactPreparedResponse{}, fmt.Errorf("exact provider HTTP status is invalid")
	}
	if status < 200 || status >= 300 {
		return ExactPreparedResponse{Disposition: ProviderResponseHTTPError}, fmt.Errorf(
			"exact provider request failed with status %d; body_bytes=%d body_sha256=%s",
			status, len(body), providerBodySHA256(body),
		)
	}
	invalid := ExactPreparedResponse{Disposition: ProviderResponseInvalidJSON}
	if !utf8.Valid(body) {
		return invalid, fmt.Errorf("exact provider response is not valid UTF-8")
	}
	if err := exactjson.ValidateObject(
		body, exactPreparedResponseWire{}, "exact raw generation response",
	); err != nil {
		return invalid, err
	}
	var wire exactPreparedResponseWire
	if err := json.Unmarshal(body, &wire); err != nil {
		return invalid, err
	}
	if _, err := parseExactProviderTimestamp(wire.CreatedAt, 9); err != nil {
		return invalid, fmt.Errorf("exact provider response created_at is not canonical UTC")
	}
	content, err := strictExactJSONObjectString(body, "response")
	if err != nil {
		return invalid, err
	}
	response := ExactPreparedResponse{
		Disposition: ProviderResponseSucceeded,
		Model:       wire.Model, Content: content, DonePresent: wire.Done != nil,
		DoneReason: wire.DoneReason, Usage: exactPreparedResponseUsage(wire),
	}
	if wire.Done != nil {
		response.Done = *wire.Done
	}
	response.UsagePresent = wire.PromptEvalCount != nil && wire.EvalCount != nil &&
		wire.TotalDuration != nil && wire.LoadDuration != nil &&
		wire.PromptEvalDuration != nil && wire.EvalDuration != nil
	if strings.TrimSpace(content) == "" {
		response.Disposition = ProviderResponseEmptyContent
		return response, fmt.Errorf("exact provider response contains no model output")
	}
	if !response.DonePresent || !response.Done || !response.UsagePresent {
		return response, fmt.Errorf("exact provider response lacks final usage or timing fields")
	}
	if err := response.Usage.ValidateSuccessful(); err != nil {
		return response, err
	}
	return response, nil
}

// ValidateExactPreparedResponseProjection proves that the normalized provider
// fields were derived from the exact captured raw response rather than supplied
// independently by a client implementation.
func ValidateExactPreparedResponseProjection(generation PreparedGeneration) error {
	if generation.ProviderResponseDisposition == ProviderResponseTransportError ||
		generation.ProviderResponseDisposition == ProviderResponseBodyLimit ||
		generation.ProviderResponseDisposition == ProviderResponseBodyReadError {
		return nil
	}
	derived, _ := DecodeExactPreparedResponse(
		generation.ProviderHTTPStatus, generation.ProviderResponseCapture,
	)
	if derived.Disposition != generation.ProviderResponseDisposition ||
		derived.Model != generation.ProviderResponseModel || derived.Content != generation.Content ||
		derived.DonePresent != generation.ProviderDonePresent || derived.Done != generation.ProviderDone ||
		derived.DoneReason != generation.ProviderDoneReason ||
		derived.UsagePresent != generation.UsagePresent || derived.Usage != generation.Usage {
		return fmt.Errorf("normalized provider response differs from its exact raw capture")
	}
	return nil
}

func exactPreparedResponseUsage(wire exactPreparedResponseWire) ProviderGenerationUsage {
	usage := ProviderGenerationUsage{}
	if wire.PromptEvalCount != nil {
		usage.PromptEvalCount = *wire.PromptEvalCount
	}
	if wire.EvalCount != nil {
		usage.EvalCount = *wire.EvalCount
	}
	if wire.TotalDuration != nil {
		usage.TotalDurationNanos = *wire.TotalDuration
	}
	if wire.LoadDuration != nil {
		usage.LoadDurationNanos = *wire.LoadDuration
	}
	if wire.PromptEvalDuration != nil {
		usage.PromptEvalDurationNanos = *wire.PromptEvalDuration
	}
	if wire.EvalDuration != nil {
		usage.EvalDurationNanos = *wire.EvalDuration
	}
	return usage
}

func strictExactJSONObjectString(raw []byte, field string) (string, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return "", err
	}
	encoded, exists := fields[field]
	if !exists || len(encoded) < 2 || encoded[0] != '"' || encoded[len(encoded)-1] != '"' {
		return "", fmt.Errorf("exact JSON field %q is not a string", field)
	}
	for index := 1; index < len(encoded)-1; index++ {
		if encoded[index] != '\\' {
			continue
		}
		index++
		if index >= len(encoded)-1 {
			return "", fmt.Errorf("exact JSON field %q has a trailing escape", field)
		}
		if encoded[index] != 'u' {
			continue
		}
		value, next, err := exactJSONUTF16Escape(encoded, index)
		if err != nil {
			return "", fmt.Errorf("exact JSON field %q: %w", field, err)
		}
		index = next
		if value >= 0xD800 && value <= 0xDBFF {
			if index+6 >= len(encoded) || encoded[index+1] != '\\' || encoded[index+2] != 'u' {
				return "", fmt.Errorf("high surrogate lacks its exact low-surrogate pair")
			}
			low, lowEnd, err := exactJSONUTF16Escape(encoded, index+2)
			if err != nil || low < 0xDC00 || low > 0xDFFF {
				return "", fmt.Errorf("high surrogate has an invalid low-surrogate pair")
			}
			index = lowEnd
		} else if value >= 0xDC00 && value <= 0xDFFF {
			return "", fmt.Errorf("low surrogate lacks a preceding high surrogate")
		}
	}
	var value string
	if err := json.Unmarshal(encoded, &value); err != nil {
		return "", err
	}
	return value, nil
}

func exactJSONUTF16Escape(encoded []byte, uIndex int) (uint16, int, error) {
	if uIndex+4 >= len(encoded) {
		return 0, 0, fmt.Errorf("Unicode escape is truncated")
	}
	raw := make([]byte, 2)
	if _, err := hex.Decode(raw, encoded[uIndex+1:uIndex+5]); err != nil {
		return 0, 0, fmt.Errorf("Unicode escape is invalid")
	}
	return uint16(raw[0])<<8 | uint16(raw[1]), uIndex + 4, nil
}
