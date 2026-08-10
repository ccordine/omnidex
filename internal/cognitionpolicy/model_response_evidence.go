package cognitionpolicy

import (
	"fmt"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
)

const (
	ModelResponseEvidenceSchemaV1 = "omnidex.cognition-model-response-evidence.v1"
	MaxModelResponseEvidenceBytes = llm.MaxExactPreparedProviderResponseBytes
)

type ModelResponseEvidenceRef struct {
	Schema string `json:"schema"`
	ID     string `json:"id"`
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
}

type ModelResponseEvidence struct {
	Ref     ModelResponseEvidenceRef
	CallID  string
	Content []byte
}

func NewModelResponseEvidence(callID, response string) (ModelResponseEvidence, error) {
	if response == "" {
		return ModelResponseEvidence{}, nil
	}
	if !validExactName(callID, 256) || len(response) > MaxModelResponseEvidenceBytes {
		return ModelResponseEvidence{}, fmt.Errorf("%w: model response evidence is outside its hard bound", ErrInvalidEvidence)
	}
	ref := modelResponseEvidenceRef(callID, response)
	value := ModelResponseEvidence{Ref: ref, CallID: callID, Content: []byte(response)}
	return value, value.Validate()
}

func modelResponseEvidenceRef(callID, response string) ModelResponseEvidenceRef {
	if response == "" {
		return ModelResponseEvidenceRef{}
	}
	ref := ModelResponseEvidenceRef{
		Schema: ModelResponseEvidenceSchemaV1,
		SHA256: policySHA256(response), Bytes: len(response),
	}
	ref.ID = modelResponseEvidenceID(callID, ref)
	return ref
}

func (ref ModelResponseEvidenceRef) ValidateFor(callID string) error {
	if ref.Schema != ModelResponseEvidenceSchemaV1 ||
		!validExactName(ref.ID, 256) || !validPolicySHA256(ref.SHA256) ||
		ref.Bytes < 1 || ref.Bytes > MaxModelResponseEvidenceBytes ||
		ref.ID != modelResponseEvidenceID(callID, ref) {
		return fmt.Errorf("%w: model response evidence reference is invalid", ErrInvalidEvidence)
	}
	return nil
}

func (evidence ModelResponseEvidence) Validate() error {
	if err := evidence.Ref.ValidateFor(evidence.CallID); err != nil {
		return err
	}
	if len(evidence.Content) != evidence.Ref.Bytes ||
		policySHA256(string(evidence.Content)) != evidence.Ref.SHA256 {
		return fmt.Errorf("%w: exact model response evidence bytes changed", ErrInvalidEvidence)
	}
	return nil
}

func (evidence ModelResponseEvidence) Clone() ModelResponseEvidence {
	evidence.Content = append([]byte(nil), evidence.Content...)
	return evidence
}

func modelResponseEvidenceID(callID string, ref ModelResponseEvidenceRef) string {
	copy := ref
	copy.ID = ""
	raw, err := exactjson.Canonical(struct {
		CallID string                   `json:"call_id"`
		Ref    ModelResponseEvidenceRef `json:"ref"`
	}{callID, copy})
	if err != nil {
		panic(fmt.Sprintf("marshal model response evidence identity: %v", err))
	}
	return "cognition_response_" + policySHA256(string(raw))
}
