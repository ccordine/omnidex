package cognitiongauntlet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/llm"
)

type embeddedSemanticSidecars struct {
	verified cognitionreplay.VerifiedBase
	values   map[string]cognitionreplay.ProjectionSidecarAuthority
	used     map[string]struct{}
}

func verifyEmbeddedSemanticReplayEvidence(
	verified cognitionreplay.VerifiedBase,
	bundle PublicInferenceBundle,
	trace productionTrace,
	values []cognitionreplay.ProjectionSidecarAuthority,
) (semanticReplaySupplement, error) {
	frozen, err := bundle.Authority.RatGeneration.Fixed.Brain.attestedBrain()
	if err != nil {
		return semanticReplaySupplement{}, err
	}
	inventory, err := collectSemanticReplayEvidence(trace, frozen)
	if err != nil {
		return semanticReplaySupplement{}, err
	}
	embedded, err := newEmbeddedSemanticSidecars(verified, values)
	if err != nil {
		return semanticReplaySupplement{}, err
	}
	var supplement semanticReplaySupplement
	for _, record := range trace.Records {
		kind := semanticPolicyEvidenceKind(record.Kind)
		if kind == "" {
			continue
		}
		metadata := inventory.policy[semanticReplayEvidenceKey(kind, record.ID)]
		raw, err := embedded.take(semanticPolicySidecarKind(kind), metadata.EvidenceID)
		if err != nil {
			return supplement, err
		}
		content, chunked, blobs, err := semanticReplayPolicyBodyContent(kind, metadata, raw)
		if err != nil {
			return supplement, err
		}
		if err := supplement.add(
			semanticPolicySidecarKind(kind), metadata.EvidenceID, content, chunked, blobs,
		); err != nil {
			return supplement, err
		}
		if err := supplement.addPolicyBody(kind, metadata.EvidenceID, raw); err != nil {
			return supplement, err
		}
	}
	identities, err := rebuildEmbeddedProviderIdentities(embedded, inventory, &supplement)
	if err != nil {
		return supplement, err
	}
	if err := supplement.setIdentities(identities); err != nil {
		return supplement, err
	}
	bootstrapRaw, err := embedded.takeOnlyKind(semanticSidecarRuntimeBrainBootstrap)
	if err != nil {
		return supplement, err
	}
	activationRaw, err := embedded.takeOnlyKind(semanticSidecarRuntimeActivation)
	if err != nil {
		return supplement, err
	}
	if err := addSemanticRuntimeSidecars(ProductionSemanticReplaySidecars{
		RuntimeBrainBootstrapEvidence:     bootstrapRaw,
		RuntimeProviderActivationEvidence: activationRaw,
	}, bundle.Authority.RatGeneration.Fixed.Brain, inventory, identities, &supplement); err != nil {
		return supplement, err
	}
	if len(embedded.used) != len(embedded.values) {
		return supplement, fmt.Errorf("semantic replay contains an unbound evidence sidecar")
	}
	if err := supplement.finish(); err != nil {
		return supplement, err
	}
	return supplement, nil
}

func newEmbeddedSemanticSidecars(
	verified cognitionreplay.VerifiedBase,
	values []cognitionreplay.ProjectionSidecarAuthority,
) (*embeddedSemanticSidecars, error) {
	result := &embeddedSemanticSidecars{
		verified: verified, values: make(map[string]cognitionreplay.ProjectionSidecarAuthority, len(values)),
		used: make(map[string]struct{}, len(values)),
	}
	for _, value := range values {
		key := semanticReplayEvidenceKey(value.Kind, value.ID)
		if _, duplicate := result.values[key]; duplicate {
			return nil, fmt.Errorf("semantic replay evidence sidecar identity is duplicated")
		}
		result.values[key] = value
	}
	return result, nil
}

func (value *embeddedSemanticSidecars) take(kind, id string) ([]byte, error) {
	key := semanticReplayEvidenceKey(kind, id)
	entry, exists := value.values[key]
	if !exists {
		return nil, fmt.Errorf("semantic replay evidence sidecar %s/%s is missing", kind, id)
	}
	if _, duplicate := value.used[key]; duplicate {
		return nil, fmt.Errorf("semantic replay evidence sidecar %s/%s was reused", kind, id)
	}
	raw, err := value.verified.ProjectionContent(entry.Content)
	if err != nil {
		return nil, fmt.Errorf("semantic replay evidence sidecar %s/%s changed: %w", kind, id, err)
	}
	value.used[key] = struct{}{}
	return raw, nil
}

func (value *embeddedSemanticSidecars) takeOnlyKind(kind string) ([]byte, error) {
	ids := make([]string, 0, 1)
	for _, item := range value.values {
		if item.Kind == kind {
			ids = append(ids, item.ID)
		}
	}
	if len(ids) != 1 {
		return nil, fmt.Errorf("semantic replay requires exactly one %s sidecar", kind)
	}
	return value.take(kind, ids[0])
}

func rebuildEmbeddedProviderIdentities(
	embedded *embeddedSemanticSidecars,
	inventory semanticReplayEvidenceInventory,
	supplement *semanticReplaySupplement,
) (map[string]llm.ProviderIdentityEvidence, error) {
	ids := make([]string, 0, len(inventory.identities))
	for id := range inventory.identities {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make(map[string]llm.ProviderIdentityEvidence, len(ids))
	for _, id := range ids {
		raw, err := embedded.take(semanticSidecarProviderIdentityManifest, id)
		if err != nil {
			return nil, err
		}
		var evidence llm.ProviderIdentityEvidence
		if err := decodeStrictJSON(bytes.TrimSuffix(raw, []byte{'\n'}), &evidence,
			"embedded provider identity manifest"); err != nil {
			return nil, err
		}
		canonical, encodeErr := json.Marshal(evidence)
		canonical = append(canonical, '\n')
		if encodeErr != nil || !bytes.Equal(raw, canonical) || evidence.Ref != inventory.identities[id] ||
			len(evidence.Operations) != 5 {
			return nil, fmt.Errorf("embedded provider identity manifest changed")
		}
		for index := range evidence.Operations {
			request, err := embedded.take(
				semanticSidecarProviderIdentityRequest, semanticIdentityBodyID(id, index),
			)
			if err != nil {
				return nil, err
			}
			response, err := embedded.take(
				semanticSidecarProviderIdentityResponse, semanticIdentityBodyID(id, index),
			)
			if err != nil {
				return nil, err
			}
			evidence.Operations[index].Request = append([]byte{}, request...)
			evidence.Operations[index].ResponseCapture = append([]byte{}, response...)
			if err := addSemanticProviderIdentityBody(
				supplement, id, index, semanticSidecarProviderIdentityRequest, request,
			); err != nil {
				return nil, err
			}
			if err := addSemanticProviderIdentityBody(
				supplement, id, index, semanticSidecarProviderIdentityResponse, response,
			); err != nil {
				return nil, err
			}
		}
		if err := evidence.Validate(); err != nil {
			return nil, fmt.Errorf("embedded provider identity evidence changed: %w", err)
		}
		manifest := evidence.Clone()
		for index := range manifest.Operations {
			manifest.Operations[index].Request = nil
			manifest.Operations[index].ResponseCapture = nil
		}
		content, chunked, blobs, err := semanticReplayProjectionContent(
			"provider-identity-manifest-"+id, manifest,
		)
		if err != nil {
			return nil, err
		}
		if err := supplement.add(
			semanticSidecarProviderIdentityManifest, id, content, chunked, blobs,
		); err != nil {
			return nil, err
		}
		result[id] = evidence.Clone()
	}
	return result, nil
}
