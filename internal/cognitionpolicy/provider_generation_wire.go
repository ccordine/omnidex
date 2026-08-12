package cognitionpolicy

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
)

type providerGenerationEvidenceWire struct {
	Schema                        providerGenerationWireBytes         `json:"schema"`
	ProviderRequestDisposition    providerGenerationWireBytes         `json:"provider_request_disposition"`
	Content                       providerGenerationWireBytes         `json:"content"`
	ProviderRequestSHA256         providerGenerationWireBytes         `json:"provider_request_sha256"`
	ProviderHTTPStatus            int                                 `json:"provider_http_status"`
	ProviderResponseDisposition   providerGenerationWireBytes         `json:"provider_response_disposition"`
	ProviderResponseComplete      bool                                `json:"provider_response_complete"`
	ProviderContentEncoding       providerContentEncodingEvidenceWire `json:"provider_content_encoding"`
	ProviderResponseBytesKnown    bool                                `json:"provider_response_bytes_known"`
	ProviderResponseSHA256        providerGenerationWireBytes         `json:"provider_response_sha256"`
	ProviderResponseBytes         int64                               `json:"provider_response_bytes"`
	ProviderResponseCaptureSHA256 providerGenerationWireBytes         `json:"provider_response_capture_sha256"`
	ProviderResponseCapturedBytes int                                 `json:"provider_response_captured_bytes"`
	ProviderResponseCapture       providerGenerationWireBytes         `json:"provider_response_capture"`
	ProviderResponseModel         providerGenerationWireBytes         `json:"provider_response_model"`
	ProviderDonePresent           bool                                `json:"provider_done_present"`
	ProviderDone                  bool                                `json:"provider_done"`
	ProviderDoneReason            providerGenerationWireBytes         `json:"provider_done_reason"`
	UsagePresent                  bool                                `json:"usage_present"`
	Usage                         llm.ProviderGenerationUsage         `json:"usage"`
	ProviderObservation           providerIdentityObservationWire     `json:"provider_observation"`
	ProviderIdentityEvidence      providerIdentityEvidenceWire        `json:"provider_identity_evidence"`
	ProviderErrorPresent          bool                                `json:"provider_error_present"`
	ProviderError                 providerGenerationWireBytes         `json:"provider_error"`
}

func encodeProviderGenerationEvidence(generation llm.PreparedGeneration) ([]byte, error) {
	return encodeProviderGenerationOutcomeEvidence(generation, "", false)
}

func encodeProviderGenerationOutcomeEvidence(
	generation llm.PreparedGeneration,
	providerError string,
	providerErrorPresent bool,
) ([]byte, error) {
	field := func(value string) providerGenerationWireBytes {
		return newProviderGenerationWireString(value, maxProviderGenerationMetadataCaptureBytes)
	}
	wire := providerGenerationEvidenceWire{
		Schema:                        field(generation.Schema),
		ProviderRequestDisposition:    field(string(generation.ProviderRequestDisposition)),
		Content:                       newProviderGenerationWireString(generation.Content, MaxModelResponseEvidenceBytes),
		ProviderRequestSHA256:         field(generation.ProviderRequestSHA256),
		ProviderHTTPStatus:            generation.ProviderHTTPStatus,
		ProviderResponseDisposition:   field(string(generation.ProviderResponseDisposition)),
		ProviderResponseComplete:      generation.ProviderResponseComplete,
		ProviderContentEncoding:       encodeProviderContentEncodingWire(generation.ProviderContentEncoding),
		ProviderResponseBytesKnown:    generation.ProviderResponseBytesKnown,
		ProviderResponseSHA256:        field(generation.ProviderResponseSHA256),
		ProviderResponseBytes:         generation.ProviderResponseBytes,
		ProviderResponseCaptureSHA256: field(generation.ProviderResponseCaptureSHA256),
		ProviderResponseCapturedBytes: generation.ProviderResponseCapturedBytes,
		ProviderResponseCapture: newProviderGenerationWireBytes(
			generation.ProviderResponseCapture, llm.MaxExactPreparedProviderResponseBytes+1,
		),
		ProviderResponseModel: field(generation.ProviderResponseModel),
		ProviderDonePresent:   generation.ProviderDonePresent, ProviderDone: generation.ProviderDone,
		ProviderDoneReason: field(generation.ProviderDoneReason),
		UsagePresent:       generation.UsagePresent, Usage: generation.Usage,
		ProviderObservation:      encodeProviderObservationWire(generation.ProviderObservation, field),
		ProviderIdentityEvidence: encodeProviderIdentityEvidenceWire(generation.ProviderIdentityEvidence, field),
		ProviderErrorPresent:     providerErrorPresent,
		ProviderError: newProviderGenerationWireString(
			providerError, maxProviderGenerationMetadataCaptureBytes,
		),
	}
	return exactjson.Canonical(wire)
}

func decodeProviderGenerationEvidence(raw []byte) (llm.PreparedGeneration, error) {
	generation, complete, err := inspectProviderGenerationEvidence(raw)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	if !complete {
		return llm.PreparedGeneration{}, fmt.Errorf("provider generation evidence contains a bounded overflow witness")
	}
	return generation, nil
}

func inspectProviderGenerationEvidence(raw []byte) (llm.PreparedGeneration, bool, error) {
	generation, _, _, complete, err := inspectProviderGenerationOutcomeEvidence(raw)
	return generation, complete, err
}

func inspectProviderGenerationOutcomeEvidence(
	raw []byte,
) (llm.PreparedGeneration, bool, string, bool, error) {
	var wire providerGenerationEvidenceWire
	if err := exactjson.ValidateObject(raw, wire, "provider generation evidence"); err != nil {
		return llm.PreparedGeneration{}, false, "", false, err
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return llm.PreparedGeneration{}, false, "", false, err
	}
	canonical, err := exactjson.Canonical(wire)
	if err != nil || !bytes.Equal(canonical, raw) {
		return llm.PreparedGeneration{}, false, "", false,
			fmt.Errorf("provider generation evidence wire is not canonical")
	}
	fields := providerGenerationWireFields(wire)
	values := make([][]byte, len(fields))
	topLevelComplete := true
	for index, item := range fields {
		value, exact, fieldErr := item.value.exact(item.limit)
		if fieldErr != nil {
			return llm.PreparedGeneration{}, false, "", false, fieldErr
		}
		values[index] = value
		topLevelComplete = topLevelComplete && exact
	}
	providerError, providerErrorComplete, err := wire.ProviderError.exact(
		maxProviderGenerationMetadataCaptureBytes,
	)
	if err != nil {
		return llm.PreparedGeneration{}, false, "", false, err
	}
	if !wire.ProviderErrorPresent && len(providerError) > 0 && providerErrorComplete {
		return llm.PreparedGeneration{}, false, "", false,
			fmt.Errorf("provider error presence differs from its exact bytes")
	}
	observation, observationComplete, err := decodeProviderObservationWire(wire.ProviderObservation)
	if err != nil {
		return llm.PreparedGeneration{}, false, "", false, err
	}
	identity, identityComplete, err := decodeProviderIdentityEvidenceWire(wire.ProviderIdentityEvidence)
	if err != nil {
		return llm.PreparedGeneration{}, false, "", false, err
	}
	contentEncoding, encodingComplete, err := decodeProviderContentEncodingWire(wire.ProviderContentEncoding)
	if err != nil {
		return llm.PreparedGeneration{}, false, "", false, err
	}
	complete := topLevelComplete && providerErrorComplete && observationComplete &&
		identityComplete && encodingComplete
	if !complete {
		return llm.PreparedGeneration{}, wire.ProviderErrorPresent, "", false, nil
	}
	return generationFromProviderWire(wire, values, contentEncoding, observation, identity),
		wire.ProviderErrorPresent, string(providerError), true, nil
}

type providerGenerationWireField struct {
	value providerGenerationWireBytes
	limit int
}

func providerGenerationWireFields(wire providerGenerationEvidenceWire) []providerGenerationWireField {
	metadata := func(value providerGenerationWireBytes) providerGenerationWireField {
		return providerGenerationWireField{value, maxProviderGenerationMetadataCaptureBytes}
	}
	return []providerGenerationWireField{metadata(wire.Schema), metadata(wire.ProviderRequestDisposition),
		{wire.Content, MaxModelResponseEvidenceBytes}, metadata(wire.ProviderRequestSHA256),
		metadata(wire.ProviderResponseDisposition), metadata(wire.ProviderResponseSHA256),
		metadata(wire.ProviderResponseCaptureSHA256),
		{wire.ProviderResponseCapture, llm.MaxExactPreparedProviderResponseBytes + 1},
		metadata(wire.ProviderResponseModel),
		metadata(wire.ProviderDoneReason)}
}

func generationFromProviderWire(
	wire providerGenerationEvidenceWire,
	values [][]byte,
	contentEncoding llm.ProviderContentEncodingEvidence,
	observation llm.ProviderIdentityObservation,
	identity llm.ProviderIdentityEvidence,
) llm.PreparedGeneration {
	return llm.PreparedGeneration{
		Schema:                     string(values[0]),
		ProviderRequestDisposition: llm.ProviderRequestDisposition(string(values[1])),
		Content:                    string(values[2]), ProviderRequestSHA256: string(values[3]),
		ProviderHTTPStatus:          wire.ProviderHTTPStatus,
		ProviderResponseDisposition: llm.ProviderResponseDisposition(string(values[4])),
		ProviderResponseComplete:    wire.ProviderResponseComplete,
		ProviderContentEncoding:     contentEncoding,
		ProviderResponseBytesKnown:  wire.ProviderResponseBytesKnown,
		ProviderResponseSHA256:      string(values[5]), ProviderResponseBytes: wire.ProviderResponseBytes,
		ProviderResponseCaptureSHA256: string(values[6]),
		ProviderResponseCapturedBytes: wire.ProviderResponseCapturedBytes,
		ProviderResponseCapture:       append([]byte{}, values[7]...),
		ProviderResponseModel:         string(values[8]), ProviderDonePresent: wire.ProviderDonePresent,
		ProviderDone: wire.ProviderDone, ProviderDoneReason: string(values[9]),
		UsagePresent: wire.UsagePresent, Usage: wire.Usage,
		ProviderObservation: observation, ProviderIdentityEvidence: identity,
	}
}
