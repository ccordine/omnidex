package cognitiongauntlet

import (
	"bytes"
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/llm"
)

func (value semanticReplaySupplement) callEvidence(
	result cognitionpolicy.CallResult,
) (cognitionpolicy.CallEvidence, []string, error) {
	evidence := cognitionpolicy.CallEvidence{}
	used := make([]string, 0, 3)
	if result.ProviderIdentityEvidence != (llm.ProviderIdentityEvidenceRef{}) {
		identity, exists := value.identity[result.ProviderIdentityEvidence.ID]
		if !exists || identity.Ref != result.ProviderIdentityEvidence {
			return evidence, nil, fmt.Errorf("semantic call result lacks exact provider identity evidence")
		}
		evidence.ProviderIdentity = identity.Clone()
	}
	if result.ResponseEvidence != (cognitionpolicy.ModelResponseEvidenceRef{}) {
		raw, key, err := value.policyBody(
			"model_response", result.ResponseEvidence.ID,
		)
		if err != nil {
			return evidence, nil, err
		}
		evidence.Response = cognitionpolicy.ModelResponseEvidence{
			Ref: result.ResponseEvidence, CallID: result.CallID, Content: raw,
		}
		used = append(used, key)
	}
	if result.ProviderGenerationEvidence != (cognitionpolicy.ProviderGenerationEvidenceRef{}) {
		raw, key, err := value.policyBody(
			"provider_generation", result.ProviderGenerationEvidence.ID,
		)
		if err != nil {
			return evidence, nil, err
		}
		evidence.ProviderGeneration = cognitionpolicy.ProviderGenerationEvidence{
			Ref: result.ProviderGenerationEvidence, CallID: result.CallID, Generation: raw,
		}
		used = append(used, key)
	}
	if result.ProviderResponseCapture != (cognitionpolicy.ProviderResponseCaptureEvidenceRef{}) {
		raw, key, err := value.policyBody(
			"provider_response_capture", result.ProviderResponseCapture.ID,
		)
		if err != nil {
			return evidence, nil, err
		}
		evidence.ProviderResponseCapture = cognitionpolicy.ProviderResponseCaptureEvidence{
			Ref: result.ProviderResponseCapture, CallID: result.CallID, Content: raw,
		}
		used = append(used, key)
	}
	return evidence, used, nil
}

func (value semanticReplaySupplement) policyBody(
	kind, id string,
) ([]byte, string, error) {
	key := semanticReplayEvidenceKey(kind, id)
	raw, exists := value.policy[key]
	if !exists {
		return nil, "", fmt.Errorf("semantic call result lacks exact %s body %q", kind, id)
	}
	return bytes.Clone(raw), key, nil
}
