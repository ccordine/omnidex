package cognitionpolicy

import (
	"bytes"
	"fmt"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
)

const (
	ProviderGenerationEvidenceSchemaV1  = "omnidex.provider-generation-evidence.v1"
	maxProviderIdentityWireCaptureBytes = maxProviderIdentityWireOperations *
		((llm.MaxProviderIdentityComponentBytes + 1) +
			(llm.MaxProviderIdentityComponentBytes + 2))
	maxProviderContentEncodingWireCaptureBytes = (1 + maxProviderIdentityWireOperations) *
		((2 * (maxProviderGenerationMetadataCaptureBytes + 1)) +
			(maxProviderContentEncodingBase64Bytes + 1))
	maxProviderGenerationMetadataWireFields = 8 + 18 + 4 +
		(6 * maxProviderIdentityWireOperations)
	maxProviderGenerationWireCaptureBytes = (MaxModelResponseEvidenceBytes + 1) +
		(llm.MaxExactPreparedProviderResponseBytes + 2) +
		maxProviderIdentityWireCaptureBytes +
		maxProviderContentEncodingWireCaptureBytes +
		(maxProviderGenerationMetadataWireFields *
			(maxProviderGenerationMetadataCaptureBytes + 1))
	// Canonical JSON base64-encodes every bounded byte witness. The fixed JSON
	// allowance covers field names, numeric values, and structural punctuation.
	// This includes six adversarial identity operations whose request, response,
	// and content-encoding fields all independently overflow their normal caps.
	MaxProviderGenerationEvidenceWireOverheadBytes = 8 * 1024 * 1024
	MaxProviderGenerationEvidenceBytes             = ((maxProviderGenerationWireCaptureBytes + 2) / 3 * 4) +
		MaxProviderGenerationEvidenceWireOverheadBytes
)

type ProviderGenerationEvidenceRef struct {
	Schema string `json:"schema"`
	ID     string `json:"id"`
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
}

type ProviderGenerationEvidence struct {
	Ref        ProviderGenerationEvidenceRef
	CallID     string
	Generation []byte
}

func NewProviderGenerationEvidence(
	callID string,
	generation llm.PreparedGeneration,
) (ProviderGenerationEvidence, error) {
	return newProviderGenerationOutcomeEvidence(callID, generation, nil)
}

func newProviderGenerationOutcomeEvidence(
	callID string,
	generation llm.PreparedGeneration,
	providerErr error,
) (ProviderGenerationEvidence, error) {
	if !validExactName(callID, 256) {
		return ProviderGenerationEvidence{}, fmt.Errorf(
			"%w: untrusted provider generation call identity is invalid", ErrInvalidEvidence,
		)
	}
	var providerError []byte
	if providerErr != nil {
		providerError = []byte(providerErr.Error())
	}
	raw, err := encodeProviderGenerationOutcomeEvidence(
		generation, providerError, providerErr != nil,
	)
	if err != nil || len(raw) < 1 || len(raw) > MaxProviderGenerationEvidenceBytes {
		return ProviderGenerationEvidence{}, fmt.Errorf(
			"%w: untrusted provider generation is outside its evidence bound", ErrInvalidEvidence,
		)
	}
	ref := providerGenerationEvidenceRef(callID, raw)
	value := ProviderGenerationEvidence{Ref: ref, CallID: callID, Generation: raw}
	return value, value.Validate()
}

func (ref ProviderGenerationEvidenceRef) ValidateFor(callID string) error {
	if ref.Schema != ProviderGenerationEvidenceSchemaV1 ||
		!validExactName(ref.ID, 256) || !validPolicySHA256(ref.SHA256) ||
		ref.Bytes < 1 || ref.Bytes > MaxProviderGenerationEvidenceBytes ||
		ref.ID != providerGenerationEvidenceID(callID, ref) {
		return fmt.Errorf("%w: provider generation evidence reference is invalid", ErrInvalidEvidence)
	}
	return nil
}

func (evidence ProviderGenerationEvidence) Validate() error {
	if err := evidence.Ref.ValidateFor(evidence.CallID); err != nil {
		return err
	}
	generation, providerErrorPresent, providerError, complete, err :=
		inspectProviderGenerationOutcomeEvidence(evidence.Generation)
	if err != nil {
		return fmt.Errorf("%w: decode exact provider generation evidence: %v", ErrInvalidEvidence, err)
	}
	if len(evidence.Generation) != evidence.Ref.Bytes ||
		policySHA256(string(evidence.Generation)) != evidence.Ref.SHA256 {
		return fmt.Errorf("%w: exact provider generation evidence changed", ErrInvalidEvidence)
	}
	if !complete {
		return nil
	}
	canonical, err := encodeProviderGenerationOutcomeEvidence(
		generation, providerError, providerErrorPresent,
	)
	if err != nil || !bytes.Equal(canonical, evidence.Generation) {
		return fmt.Errorf("%w: exact provider generation evidence changed", ErrInvalidEvidence)
	}
	return nil
}

func (evidence ProviderGenerationEvidence) Clone() ProviderGenerationEvidence {
	evidence.Generation = append([]byte(nil), evidence.Generation...)
	return evidence
}

func providerGenerationEvidenceRef(callID string, raw []byte) ProviderGenerationEvidenceRef {
	ref := ProviderGenerationEvidenceRef{
		Schema: ProviderGenerationEvidenceSchemaV1,
		SHA256: policySHA256(string(raw)), Bytes: len(raw),
	}
	ref.ID = providerGenerationEvidenceID(callID, ref)
	return ref
}

func providerGenerationEvidenceID(callID string, ref ProviderGenerationEvidenceRef) string {
	copy := ref
	copy.ID = ""
	raw, err := exactjson.Canonical(struct {
		CallID string                        `json:"call_id"`
		Ref    ProviderGenerationEvidenceRef `json:"ref"`
	}{callID, copy})
	if err != nil {
		panic(fmt.Sprintf("marshal provider generation evidence identity: %v", err))
	}
	return "provider_generation_" + policySHA256(string(raw))
}

type CallEvidence struct {
	Response                ModelResponseEvidence
	ProviderIdentity        llm.ProviderIdentityEvidence
	ProviderResponseCapture ProviderResponseCaptureEvidence
	ProviderGeneration      ProviderGenerationEvidence
}

func (evidence CallEvidence) Clone() CallEvidence {
	evidence.Response = evidence.Response.Clone()
	evidence.ProviderIdentity = evidence.ProviderIdentity.Clone()
	evidence.ProviderResponseCapture = evidence.ProviderResponseCapture.Clone()
	evidence.ProviderGeneration = evidence.ProviderGeneration.Clone()
	return evidence
}
