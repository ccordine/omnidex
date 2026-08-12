package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

type semanticReplayEvidenceInventory struct {
	policy             map[string]semanticPolicyEvidence
	identities         map[string]llm.ProviderIdentityEvidenceRef
	initialBootstrap   queue.CognitionBrainBootstrapTrace
	initialObservation cognitionpolicy.ProviderProcessObservation
}

func collectSemanticReplayEvidence(
	trace productionTrace,
	frozen cognitionpolicy.AttestedBrain,
) (semanticReplayEvidenceInventory, error) {
	result := semanticReplayEvidenceInventory{
		policy:     make(map[string]semanticPolicyEvidence),
		identities: make(map[string]llm.ProviderIdentityEvidenceRef),
	}
	wantedPolicy := make(map[string]semanticPolicyEvidence)
	for _, record := range trace.Records {
		switch record.Kind {
		case "policy_response_evidence", "policy_provider_generation_evidence",
			"policy_provider_response_capture":
			value, err := decodeSemanticPolicyEvidence(record)
			if err != nil {
				return result, err
			}
			key := semanticReplayEvidenceKey(value.EvidenceKind, value.EvidenceID)
			if _, duplicate := result.policy[key]; duplicate {
				return result, fmt.Errorf("semantic policy evidence metadata is duplicated")
			}
			result.policy[key] = value
		case "policy_attempt":
			var value cognitionpolicy.CallAttempt
			if err := decodeProductionPayload(record.Payload, &value, "semantic evidence policy attempt"); err != nil {
				return result, err
			}
			if err := result.addIdentity(value.ProviderProcessActivation.Evidence); err != nil {
				return result, err
			}
		case "policy_result":
			var value cognitionpolicy.CallResult
			if err := decodeProductionPayload(record.Payload, &value, "semantic evidence policy result"); err != nil {
				return result, err
			}
			if err := collectPolicyResultEvidence(value, wantedPolicy, &result); err != nil {
				return result, err
			}
		case queue.CognitionTraceKindProviderBrainBootstrap:
			var value queue.CognitionBrainBootstrapTrace
			if err := decodeProductionPayload(record.Payload, &value, "semantic evidence Brain bootstrap"); err != nil ||
				value.Validate() != nil {
				return result, fmt.Errorf("semantic Brain bootstrap evidence authority is invalid: %v", err)
			}
			if err := result.addIdentity(value.Evidence); err != nil {
				return result, err
			}
			if value.Source == queue.CognitionBrainBootstrapEpisodeStart {
				if result.initialBootstrap.Source != "" {
					return result, fmt.Errorf("semantic replay has duplicate initial Brain bootstrap evidence")
				}
				result.initialBootstrap = value
			}
		case "provider_process_observation":
			var value cognitionpolicy.ProviderProcessObservation
			if err := decodeProductionPayload(record.Payload, &value, "semantic evidence provider observation"); err != nil ||
				value.ValidateFor(frozen) != nil {
				return result, fmt.Errorf("semantic provider observation evidence authority is invalid: %v", err)
			}
			if err := result.addIdentity(value.Observation.Evidence); err != nil {
				return result, err
			}
			if record.Sequence == 1 {
				if result.initialObservation.ID != "" {
					return result, fmt.Errorf("semantic replay has duplicate initial provider observation evidence")
				}
				result.initialObservation = value
			}
		case "provider_activation_failure":
			var value cognitionpolicy.ProviderProcessFailureReceipt
			if err := decodeProductionPayload(record.Payload, &value, "semantic evidence provider failure"); err != nil {
				return result, err
			}
			if err := result.addIdentity(value.Evidence); err != nil {
				return result, err
			}
		}
	}
	initialStable, initialStableErr := result.initialBootstrap.Brain.StableAuthority()
	frozenStable, frozenStableErr := frozen.StableAuthority()
	if result.initialBootstrap.Source == "" || result.initialObservation.ID == "" ||
		initialStableErr != nil || frozenStableErr != nil || initialStable != frozenStable ||
		result.initialBootstrap.Evidence != result.initialBootstrap.Brain.BootstrapObservation.Evidence ||
		result.initialBootstrap.Evidence != frozen.BootstrapObservation.Evidence {
		return result, fmt.Errorf("semantic replay lacks exact initial provider evidence authorities")
	}
	if len(wantedPolicy) != len(result.policy) {
		return result, fmt.Errorf("semantic policy evidence metadata is not exact and exhaustive")
	}
	for key, want := range wantedPolicy {
		got, exists := result.policy[key]
		if !exists || got != want {
			return result, fmt.Errorf("semantic policy evidence metadata differs from its call result")
		}
	}
	return result, nil
}

func collectPolicyResultEvidence(
	value cognitionpolicy.CallResult,
	wanted map[string]semanticPolicyEvidence,
	inventory *semanticReplayEvidenceInventory,
) error {
	if value.ProviderIdentityEvidence != (llm.ProviderIdentityEvidenceRef{}) {
		if err := inventory.addIdentity(value.ProviderIdentityEvidence); err != nil {
			return err
		}
	}
	values := []struct {
		kind string
		ref  any
	}{
		{"model_response", value.ResponseEvidence},
		{"provider_generation", value.ProviderGenerationEvidence},
		{"provider_response_capture", value.ProviderResponseCapture},
	}
	for _, item := range values {
		metadata, present, err := semanticPolicyMetadataForRef(item.kind, value.CallID, item.ref)
		if err != nil {
			return err
		}
		if !present {
			continue
		}
		key := semanticReplayEvidenceKey(item.kind, metadata.EvidenceID)
		if prior, duplicate := wanted[key]; duplicate && prior != metadata {
			return fmt.Errorf("semantic call results conflict on policy evidence")
		}
		wanted[key] = metadata
	}
	return nil
}

func (value *semanticReplayEvidenceInventory) addIdentity(
	ref llm.ProviderIdentityEvidenceRef,
) error {
	if err := ref.Validate(); err != nil {
		return fmt.Errorf("semantic provider identity evidence reference is invalid: %w", err)
	}
	if prior, exists := value.identities[ref.ID]; exists && prior != ref {
		return fmt.Errorf("semantic provider identity evidence reference conflicts")
	}
	value.identities[ref.ID] = ref
	return nil
}

func semanticReplayEvidenceKey(kind, id string) string { return kind + "\x00" + id }

func semanticPolicyMetadataForRef(
	kind string,
	callID string,
	ref any,
) (semanticPolicyEvidence, bool, error) {
	metadata := semanticPolicyEvidence{
		Schema: "omnidex.cognition-policy-evidence-trace.v1", EvidenceKind: kind,
		CallID: callID,
	}
	var absent bool
	switch value := ref.(type) {
	case cognitionpolicy.ModelResponseEvidenceRef:
		absent = value == (cognitionpolicy.ModelResponseEvidenceRef{})
		if !absent {
			if err := value.ValidateFor(callID); err != nil {
				return metadata, false, err
			}
			metadata.EvidenceID, metadata.ContentSHA256, metadata.Bytes = value.ID, value.SHA256, value.Bytes
		}
	case cognitionpolicy.ProviderGenerationEvidenceRef:
		absent = value == (cognitionpolicy.ProviderGenerationEvidenceRef{})
		if !absent {
			if err := value.ValidateFor(callID); err != nil {
				return metadata, false, err
			}
			metadata.EvidenceID, metadata.ContentSHA256, metadata.Bytes = value.ID, value.SHA256, value.Bytes
		}
	case cognitionpolicy.ProviderResponseCaptureEvidenceRef:
		absent = value == (cognitionpolicy.ProviderResponseCaptureEvidenceRef{})
		if !absent {
			if err := value.ValidateFor(callID); err != nil {
				return metadata, false, err
			}
			metadata.EvidenceID, metadata.ContentSHA256, metadata.Bytes = value.ID, value.SHA256, value.Bytes
		}
	default:
		return metadata, false, fmt.Errorf("semantic policy evidence reference type is unregistered")
	}
	if absent {
		return semanticPolicyEvidence{}, false, nil
	}
	raw, err := exactjson.Canonical(ref)
	if err != nil {
		return metadata, false, err
	}
	metadata.ReferenceJSONSHA256 = digestExactBytes(raw)
	return metadata, true, nil
}
