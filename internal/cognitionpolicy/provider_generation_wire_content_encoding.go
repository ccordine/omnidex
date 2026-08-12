package cognitionpolicy

import "github.com/gryph/omnidex/internal/llm"

const maxProviderContentEncodingBase64Bytes = llm.MaxProviderContentEncodingBase64Bytes

// providerContentEncodingEvidenceWire keeps every string in an untrusted
// content-encoding receipt behind the same total bounded-byte witness used by
// the rest of provider-generation evidence.
type providerContentEncodingEvidenceWire struct {
	Schema         providerGenerationWireBytes `json:"schema"`
	Values         int                         `json:"values"`
	Complete       bool                        `json:"complete"`
	SHA256         providerGenerationWireBytes `json:"sha256"`
	Bytes          int64                       `json:"bytes"`
	CapturedBase64 providerGenerationWireBytes `json:"captured_base64"`
	CapturedBytes  int                         `json:"captured_bytes"`
	Uncompressed   bool                        `json:"uncompressed"`
}

func encodeProviderContentEncodingWire(
	value llm.ProviderContentEncodingEvidence,
) providerContentEncodingEvidenceWire {
	metadata := func(raw string) providerGenerationWireBytes {
		return newProviderGenerationWireString(raw, maxProviderGenerationMetadataCaptureBytes)
	}
	return providerContentEncodingEvidenceWire{
		Schema: metadata(value.Schema), Values: value.Values, Complete: value.Complete,
		SHA256: metadata(value.SHA256), Bytes: value.Bytes,
		CapturedBase64: newProviderGenerationWireString(
			value.CapturedBase64, maxProviderContentEncodingBase64Bytes,
		),
		CapturedBytes: value.CapturedBytes, Uncompressed: value.Uncompressed,
	}
}

func decodeProviderContentEncodingWire(
	wire providerContentEncodingEvidenceWire,
) (llm.ProviderContentEncodingEvidence, bool, error) {
	schema, schemaExact, err := wire.Schema.exact(maxProviderGenerationMetadataCaptureBytes)
	if err != nil {
		return llm.ProviderContentEncodingEvidence{}, false, err
	}
	sha, shaExact, err := wire.SHA256.exact(maxProviderGenerationMetadataCaptureBytes)
	if err != nil {
		return llm.ProviderContentEncodingEvidence{}, false, err
	}
	capture, captureExact, err := wire.CapturedBase64.exact(maxProviderContentEncodingBase64Bytes)
	if err != nil {
		return llm.ProviderContentEncodingEvidence{}, false, err
	}
	if !schemaExact || !shaExact || !captureExact {
		return llm.ProviderContentEncodingEvidence{}, false, nil
	}
	return llm.ProviderContentEncodingEvidence{
		Schema: string(schema), Values: wire.Values, Complete: wire.Complete,
		SHA256: string(sha), Bytes: wire.Bytes, CapturedBase64: string(capture),
		CapturedBytes: wire.CapturedBytes, Uncompressed: wire.Uncompressed,
	}, true, nil
}
