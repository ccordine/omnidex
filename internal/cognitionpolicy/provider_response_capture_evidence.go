package cognitionpolicy

import (
	"fmt"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
)

const ProviderResponseCaptureEvidenceSchemaV1 = "omnidex.provider-response-capture-evidence.v1"

type ProviderResponseCaptureEvidenceRef struct {
	Schema string `json:"schema"`
	ID     string `json:"id"`
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
}

type ProviderResponseCaptureEvidence struct {
	Ref     ProviderResponseCaptureEvidenceRef
	CallID  string
	Content []byte
}

func NewProviderResponseCaptureEvidence(
	callID string,
	content []byte,
) (ProviderResponseCaptureEvidence, error) {
	if !validExactName(callID, 256) ||
		len(content) > llm.MaxExactPreparedProviderResponseBytes+1 {
		return ProviderResponseCaptureEvidence{}, fmt.Errorf(
			"%w: provider response capture is outside its hard bound", ErrInvalidEvidence,
		)
	}
	ref := providerResponseCaptureEvidenceRef(callID, content)
	value := ProviderResponseCaptureEvidence{
		Ref: ref, CallID: callID, Content: append([]byte(nil), content...),
	}
	return value, value.Validate()
}

func (ref ProviderResponseCaptureEvidenceRef) ValidateFor(callID string) error {
	if ref.Schema != ProviderResponseCaptureEvidenceSchemaV1 ||
		!validExactName(ref.ID, 256) || !validPolicySHA256(ref.SHA256) ||
		ref.Bytes < 0 || ref.Bytes > llm.MaxExactPreparedProviderResponseBytes+1 ||
		ref.ID != providerResponseCaptureEvidenceID(callID, ref) {
		return fmt.Errorf("%w: provider response capture reference is invalid", ErrInvalidEvidence)
	}
	return nil
}

func (evidence ProviderResponseCaptureEvidence) Validate() error {
	if err := evidence.Ref.ValidateFor(evidence.CallID); err != nil {
		return err
	}
	if len(evidence.Content) != evidence.Ref.Bytes ||
		policySHA256(string(evidence.Content)) != evidence.Ref.SHA256 {
		return fmt.Errorf("%w: provider response capture bytes changed", ErrInvalidEvidence)
	}
	return nil
}

func (evidence ProviderResponseCaptureEvidence) Clone() ProviderResponseCaptureEvidence {
	evidence.Content = append([]byte(nil), evidence.Content...)
	return evidence
}

func providerResponseCaptureEvidenceRef(
	callID string,
	content []byte,
) ProviderResponseCaptureEvidenceRef {
	ref := ProviderResponseCaptureEvidenceRef{
		Schema: ProviderResponseCaptureEvidenceSchemaV1,
		SHA256: policySHA256(string(content)), Bytes: len(content),
	}
	ref.ID = providerResponseCaptureEvidenceID(callID, ref)
	return ref
}

func providerResponseCaptureEvidenceID(
	callID string,
	ref ProviderResponseCaptureEvidenceRef,
) string {
	copy := ref
	copy.ID = ""
	raw, err := exactjson.Canonical(struct {
		CallID string                             `json:"call_id"`
		Ref    ProviderResponseCaptureEvidenceRef `json:"ref"`
	}{callID, copy})
	if err != nil {
		panic(fmt.Sprintf("marshal provider response capture identity: %v", err))
	}
	return "provider_response_capture_" + policySHA256(string(raw))
}
