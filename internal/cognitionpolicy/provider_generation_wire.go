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
	ProviderRequestDispatched     bool                                `json:"provider_request_dispatched"`
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
	return encodeProviderGenerationOutcomeEvidence(generation, nil, false)
}

func encodeProviderGenerationOutcomeEvidence(
	generation llm.PreparedGeneration,
	providerError []byte,
	providerErrorPresent bool,
) ([]byte, error) {
	field := func(value string) providerGenerationWireBytes {
		return newProviderGenerationWireBytes([]byte(value), maxProviderGenerationMetadataCaptureBytes)
	}
	wire := providerGenerationEvidenceWire{
		Schema: field(generation.Schema), ProviderRequestDispatched: generation.ProviderRequestDispatched,
		Content:                       newProviderGenerationWireBytes([]byte(generation.Content), MaxModelResponseEvidenceBytes),
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
		ProviderError: newProviderGenerationWireBytes(
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
) (llm.PreparedGeneration, bool, []byte, bool, error) {
	var wire providerGenerationEvidenceWire
	if err := exactjson.ValidateObject(raw, wire, "provider generation evidence"); err != nil {
		return llm.PreparedGeneration{}, false, nil, false, err
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return llm.PreparedGeneration{}, false, nil, false, err
	}
	canonical, err := exactjson.Canonical(wire)
	if err != nil || !bytes.Equal(canonical, raw) {
		return llm.PreparedGeneration{}, false, nil, false,
			fmt.Errorf("provider generation evidence wire is not canonical")
	}
	fields := providerGenerationWireFields(wire)
	values := make([][]byte, len(fields))
	complete := true
	for index, item := range fields {
		value, exact, fieldErr := item.value.exact(item.limit)
		if fieldErr != nil {
			return llm.PreparedGeneration{}, false, nil, false, fieldErr
		}
		values[index] = value
		complete = complete && exact
	}
	if !complete {
		return llm.PreparedGeneration{}, wire.ProviderErrorPresent, nil, false, nil
	}
	providerError, providerErrorComplete, err := wire.ProviderError.exact(
		maxProviderGenerationMetadataCaptureBytes,
	)
	if err != nil {
		return llm.PreparedGeneration{}, false, nil, false, err
	}
	if !wire.ProviderErrorPresent && len(providerError) > 0 && providerErrorComplete {
		return llm.PreparedGeneration{}, false, nil, false,
			fmt.Errorf("provider error presence differs from its exact bytes")
	}
	if !providerErrorComplete {
		return llm.PreparedGeneration{}, wire.ProviderErrorPresent, nil, false, nil
	}
	observation, observationComplete, err := decodeProviderObservationWire(wire.ProviderObservation)
	if err != nil || !observationComplete {
		return llm.PreparedGeneration{}, false, nil, false, err
	}
	identity, identityComplete, err := decodeProviderIdentityEvidenceWire(wire.ProviderIdentityEvidence)
	if err != nil || !identityComplete {
		return llm.PreparedGeneration{}, false, nil, false, err
	}
	contentEncoding, encodingComplete, err := decodeProviderContentEncodingWire(wire.ProviderContentEncoding)
	if err != nil || !encodingComplete {
		return llm.PreparedGeneration{}, false, nil, false, err
	}
	return generationFromProviderWire(wire, values, contentEncoding, observation, identity),
		wire.ProviderErrorPresent, providerError, true, nil
}

type providerGenerationWireField struct {
	value providerGenerationWireBytes
	limit int
}

func providerGenerationWireFields(wire providerGenerationEvidenceWire) []providerGenerationWireField {
	metadata := func(value providerGenerationWireBytes) providerGenerationWireField {
		return providerGenerationWireField{value, maxProviderGenerationMetadataCaptureBytes}
	}
	return []providerGenerationWireField{metadata(wire.Schema),
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
		Schema: string(values[0]), ProviderRequestDispatched: wire.ProviderRequestDispatched,
		Content: string(values[1]), ProviderRequestSHA256: string(values[2]),
		ProviderHTTPStatus:          wire.ProviderHTTPStatus,
		ProviderResponseDisposition: llm.ProviderResponseDisposition(string(values[3])),
		ProviderResponseComplete:    wire.ProviderResponseComplete,
		ProviderContentEncoding:     contentEncoding,
		ProviderResponseBytesKnown:  wire.ProviderResponseBytesKnown,
		ProviderResponseSHA256:      string(values[4]), ProviderResponseBytes: wire.ProviderResponseBytes,
		ProviderResponseCaptureSHA256: string(values[5]),
		ProviderResponseCapturedBytes: wire.ProviderResponseCapturedBytes,
		ProviderResponseCapture:       append([]byte(nil), values[6]...),
		ProviderResponseModel:         string(values[7]), ProviderDonePresent: wire.ProviderDonePresent,
		ProviderDone: wire.ProviderDone, ProviderDoneReason: string(values[8]),
		UsagePresent: wire.UsagePresent, Usage: wire.Usage,
		ProviderObservation: observation, ProviderIdentityEvidence: identity,
	}
}
