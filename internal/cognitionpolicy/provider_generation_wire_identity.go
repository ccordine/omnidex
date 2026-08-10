package cognitionpolicy

import (
	"fmt"

	"github.com/gryph/omnidex/internal/llm"
)

const maxProviderIdentityWireOperations = 6

type providerIdentityEvidenceWire struct {
	Schema             providerGenerationWireBytes             `json:"schema"`
	RefSchema          providerGenerationWireBytes             `json:"ref_schema"`
	RefID              providerGenerationWireBytes             `json:"ref_id"`
	RefSHA256          providerGenerationWireBytes             `json:"ref_sha256"`
	RefBytes           int                                     `json:"ref_bytes"`
	OriginalOperations int                                     `json:"original_operations"`
	Operations         []providerIdentityOperationEvidenceWire `json:"operations"`
}

type providerIdentityOperationEvidenceWire struct {
	Operation            providerGenerationWireBytes `json:"operation"`
	Method               providerGenerationWireBytes `json:"method"`
	Endpoint             providerGenerationWireBytes `json:"endpoint"`
	RequestDispatched    bool                        `json:"request_dispatched"`
	RequestSHA256        providerGenerationWireBytes `json:"request_sha256"`
	RequestBytes         int                         `json:"request_bytes"`
	Request              providerGenerationWireBytes `json:"request"`
	HTTPStatus           int                         `json:"http_status"`
	Disposition          providerGenerationWireBytes `json:"disposition"`
	ResponseComplete     bool                        `json:"response_complete"`
	ContentEncodingCount int                         `json:"content_encoding_count"`
	ContentEncoding      providerGenerationWireBytes `json:"content_encoding"`
	ResponseUncompressed bool                        `json:"response_uncompressed"`
	ResponseSHA256       providerGenerationWireBytes `json:"response_sha256"`
	ResponseBytes        int                         `json:"response_bytes"`
	ResponseCapture      providerGenerationWireBytes `json:"response_capture"`
}

func encodeProviderIdentityEvidenceWire(
	evidence llm.ProviderIdentityEvidence,
	field func(string) providerGenerationWireBytes,
) providerIdentityEvidenceWire {
	count := len(evidence.Operations)
	captured := count
	if captured > maxProviderIdentityWireOperations {
		captured = maxProviderIdentityWireOperations
	}
	operations := make([]providerIdentityOperationEvidenceWire, captured)
	for index := range operations {
		operation := evidence.Operations[index]
		operations[index] = providerIdentityOperationEvidenceWire{
			Operation: field(string(operation.Operation)), Method: field(operation.Method),
			Endpoint: field(operation.Endpoint), RequestDispatched: operation.RequestDispatched,
			RequestSHA256: field(operation.RequestSHA256), RequestBytes: operation.RequestBytes,
			Request: newProviderGenerationWireBytes(
				operation.Request, llm.MaxProviderIdentityComponentBytes,
			),
			HTTPStatus: operation.HTTPStatus, Disposition: field(string(operation.Disposition)),
			ResponseComplete:     operation.ResponseComplete,
			ContentEncodingCount: operation.ContentEncodingCount,
			ContentEncoding:      field(operation.ContentEncoding),
			ResponseUncompressed: operation.ResponseUncompressed,
			ResponseSHA256:       field(operation.ResponseSHA256), ResponseBytes: operation.ResponseBytes,
			ResponseCapture: newProviderGenerationWireBytes(
				operation.ResponseCapture, llm.MaxProviderIdentityComponentBytes+1,
			),
		}
	}
	return providerIdentityEvidenceWire{
		Schema: field(evidence.Schema), RefSchema: field(evidence.Ref.Schema),
		RefID: field(evidence.Ref.ID), RefSHA256: field(evidence.Ref.SHA256),
		RefBytes: evidence.Ref.Bytes, OriginalOperations: count, Operations: operations,
	}
}

func decodeProviderIdentityEvidenceWire(
	wire providerIdentityEvidenceWire,
) (llm.ProviderIdentityEvidence, bool, error) {
	if wire.OriginalOperations < 0 || len(wire.Operations) > maxProviderIdentityWireOperations ||
		(wire.OriginalOperations <= maxProviderIdentityWireOperations &&
			wire.OriginalOperations != len(wire.Operations)) ||
		(wire.OriginalOperations > maxProviderIdentityWireOperations &&
			len(wire.Operations) != maxProviderIdentityWireOperations) {
		return llm.ProviderIdentityEvidence{}, false,
			fmt.Errorf("provider identity evidence operation count is invalid")
	}
	metadata := []providerGenerationWireBytes{
		wire.Schema, wire.RefSchema, wire.RefID, wire.RefSHA256,
	}
	metaValues := make([]string, len(metadata))
	complete := wire.OriginalOperations == 5 && len(wire.Operations) == 5
	for index, field := range metadata {
		raw, exact, err := field.exact(maxProviderGenerationMetadataCaptureBytes)
		if err != nil {
			return llm.ProviderIdentityEvidence{}, false, err
		}
		complete = complete && exact
		metaValues[index] = string(raw)
	}
	operations := make([]llm.ProviderIdentityOperationEvidence, len(wire.Operations))
	for index, operation := range wire.Operations {
		decoded, exact, err := decodeProviderIdentityOperationWire(operation)
		if err != nil {
			return llm.ProviderIdentityEvidence{}, false, err
		}
		complete = complete && exact
		operations[index] = decoded
	}
	if !complete {
		return llm.ProviderIdentityEvidence{}, false, nil
	}
	return llm.ProviderIdentityEvidence{
		Schema: metaValues[0],
		Ref: llm.ProviderIdentityEvidenceRef{
			Schema: metaValues[1], ID: metaValues[2], SHA256: metaValues[3], Bytes: wire.RefBytes,
		},
		Operations: operations,
	}, true, nil
}

func decodeProviderIdentityOperationWire(
	wire providerIdentityOperationEvidenceWire,
) (llm.ProviderIdentityOperationEvidence, bool, error) {
	fields := []struct {
		value providerGenerationWireBytes
		limit int
	}{
		{wire.Operation, maxProviderGenerationMetadataCaptureBytes},
		{wire.Method, maxProviderGenerationMetadataCaptureBytes},
		{wire.Endpoint, maxProviderGenerationMetadataCaptureBytes},
		{wire.RequestSHA256, maxProviderGenerationMetadataCaptureBytes},
		{wire.Request, llm.MaxProviderIdentityComponentBytes},
		{wire.Disposition, maxProviderGenerationMetadataCaptureBytes},
		{wire.ContentEncoding, maxProviderGenerationMetadataCaptureBytes},
		{wire.ResponseSHA256, maxProviderGenerationMetadataCaptureBytes},
		{wire.ResponseCapture, llm.MaxProviderIdentityComponentBytes + 1},
	}
	values := make([][]byte, len(fields))
	complete := true
	for index, field := range fields {
		raw, exact, err := field.value.exact(field.limit)
		if err != nil {
			return llm.ProviderIdentityOperationEvidence{}, false, err
		}
		complete = complete && exact
		values[index] = raw
	}
	if !complete {
		return llm.ProviderIdentityOperationEvidence{}, false, nil
	}
	return llm.ProviderIdentityOperationEvidence{
		Operation: llm.ProviderIdentityOperation(string(values[0])), Method: string(values[1]),
		Endpoint: string(values[2]), RequestDispatched: wire.RequestDispatched,
		RequestSHA256: string(values[3]), RequestBytes: wire.RequestBytes,
		Request: append([]byte(nil), values[4]...), HTTPStatus: wire.HTTPStatus,
		Disposition:          llm.ProviderIdentityOperationDisposition(string(values[5])),
		ResponseComplete:     wire.ResponseComplete,
		ContentEncodingCount: wire.ContentEncodingCount,
		ContentEncoding:      string(values[6]), ResponseUncompressed: wire.ResponseUncompressed,
		ResponseSHA256: string(values[7]), ResponseBytes: wire.ResponseBytes,
		ResponseCapture: append([]byte(nil), values[8]...),
	}, true, nil
}
