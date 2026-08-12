package cognitiongauntlet

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/llm"
)

const (
	semanticSidecarPolicyModelResponse      = "policy.model_response"
	semanticSidecarPolicyGeneration         = "policy.provider_generation"
	semanticSidecarPolicyResponseCapture    = "policy.provider_response_capture"
	semanticSidecarProviderIdentityManifest = "provider_identity.manifest"
	semanticSidecarProviderIdentityRequest  = "provider_identity.request"
	semanticSidecarProviderIdentityResponse = "provider_identity.response"
	semanticSidecarRuntimeBrainBootstrap    = "runtime.brain_bootstrap"
	semanticSidecarRuntimeActivation        = "runtime.provider_activation"
)

type semanticReplaySupplement struct {
	sidecars []cognitionreplay.ProjectionSidecarAuthority
	chunked  []cognitionreplay.ChunkedBlobBinding
	blobs    []cognitionreplay.Blob
	policy   map[string][]byte
	identity map[string]llm.ProviderIdentityEvidence
}

func (value *semanticReplaySupplement) addPolicyBody(
	kind, id string,
	raw []byte,
) error {
	if value.policy == nil {
		value.policy = make(map[string][]byte)
	}
	key := semanticReplayEvidenceKey(kind, id)
	if _, duplicate := value.policy[key]; duplicate {
		return fmt.Errorf("semantic replay policy body is duplicated")
	}
	value.policy[key] = bytes.Clone(raw)
	return nil
}

func (value *semanticReplaySupplement) setIdentities(
	identities map[string]llm.ProviderIdentityEvidence,
) error {
	if value.identity != nil {
		return fmt.Errorf("semantic replay provider identities were already bound")
	}
	value.identity = make(map[string]llm.ProviderIdentityEvidence, len(identities))
	for id, evidence := range identities {
		value.identity[id] = evidence.Clone()
	}
	return nil
}

func (value *semanticReplaySupplement) add(
	kind string,
	id string,
	content cognitionreplay.ProjectionContentAuthority,
	chunked []cognitionreplay.ChunkedBlobBinding,
	blobs []cognitionreplay.Blob,
) error {
	if kind == "" || id == "" || content.Validate() != nil {
		return fmt.Errorf("semantic replay evidence sidecar authority is invalid")
	}
	key := semanticReplayEvidenceKey(kind, id)
	for _, prior := range value.sidecars {
		if semanticReplayEvidenceKey(prior.Kind, prior.ID) == key {
			return fmt.Errorf("semantic replay evidence sidecar authority is duplicated")
		}
	}
	value.sidecars = append(value.sidecars, cognitionreplay.ProjectionSidecarAuthority{
		Kind: kind, ID: id, Content: content,
	})
	value.chunked = append(value.chunked, chunked...)
	value.blobs = append(value.blobs, blobs...)
	return nil
}

func (value *semanticReplaySupplement) finish() error {
	sort.Slice(value.sidecars, func(left, right int) bool {
		return semanticReplayEvidenceKey(value.sidecars[left].Kind, value.sidecars[left].ID) <
			semanticReplayEvidenceKey(value.sidecars[right].Kind, value.sidecars[right].ID)
	})
	var err error
	value.blobs, err = uniqueReplayBlobs(value.blobs)
	return err
}

func semanticPolicySidecarKind(kind string) string {
	switch kind {
	case "model_response":
		return semanticSidecarPolicyModelResponse
	case "provider_generation":
		return semanticSidecarPolicyGeneration
	case "provider_response_capture":
		return semanticSidecarPolicyResponseCapture
	default:
		return ""
	}
}

func semanticIdentityBodyID(evidenceID string, operation int) string {
	return fmt.Sprintf("%s/%d", evidenceID, operation)
}
