package cognitiongauntlet

import (
	"bytes"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionreplay"
	"github.com/gryph/omnidex/internal/llm"
)

func addSemanticRuntimeSidecars(
	raw ProductionSemanticReplaySidecars,
	frozen BrainFingerprint,
	inventory semanticReplayEvidenceInventory,
	identities map[string]llm.ProviderIdentityEvidence,
	supplement *semanticReplaySupplement,
) error {
	if err := raw.validate(); err != nil {
		return err
	}
	bootstrap, err := decodeSemanticRuntimeBootstrap(raw.RuntimeBrainBootstrapEvidence, frozen)
	if err != nil {
		return err
	}
	wantBootstrap, exists := identities[bootstrap.BootstrapEvidence.Ref.ID]
	if !exists || bootstrap.AttestedBrain != inventory.initialBootstrap.Brain ||
		bootstrap.BootstrapEvidence.Ref != inventory.initialBootstrap.Evidence ||
		!reflect.DeepEqual(bootstrap.BootstrapEvidence, wantBootstrap) {
		return fmt.Errorf("runtime Brain bootstrap sidecar differs from sealed queue evidence")
	}
	activation, err := decodeSemanticRuntimeActivation(
		raw.RuntimeProviderActivationEvidence, frozen,
	)
	if err != nil {
		return err
	}
	wantActivation, exists := identities[activation.IdentityEvidence.Ref.ID]
	if !exists || activation.Receipt != inventory.initialObservation ||
		activation.IdentityEvidence.Ref != inventory.initialObservation.Observation.Evidence ||
		!reflect.DeepEqual(activation.IdentityEvidence, wantActivation) ||
		inventory.initialBootstrap.Actor != inventory.initialObservation.Actor ||
		inventory.initialBootstrap.EpisodeID != inventory.initialObservation.EpisodeID {
		return fmt.Errorf("runtime provider activation sidecar differs from sealed queue evidence")
	}
	if err := addSemanticRuntimeContent(
		supplement, semanticSidecarRuntimeBrainBootstrap,
		"runtime_brain_bootstrap_evidence_"+digestExactBytes(raw.RuntimeBrainBootstrapEvidence),
		raw.RuntimeBrainBootstrapEvidence,
	); err != nil {
		return err
	}
	return addSemanticRuntimeContent(
		supplement, semanticSidecarRuntimeActivation,
		"runtime_provider_activation_evidence_"+digestExactBytes(raw.RuntimeProviderActivationEvidence),
		raw.RuntimeProviderActivationEvidence,
	)
}

func decodeSemanticRuntimeBootstrap(
	raw []byte,
	frozen BrainFingerprint,
) (bootstrapValue cognitionpolicy.BrainBootstrap, err error) {
	var artifact runtimeBrainBootstrapEvidenceArtifact
	if err := decodeStrictJSON(raw, &artifact, "semantic runtime Brain bootstrap sidecar"); err != nil {
		return bootstrapValue, err
	}
	canonical, err := encodeRuntimeBrainBootstrapEvidenceArtifact(artifact)
	if err != nil || !bytes.Equal(raw, canonical) {
		return bootstrapValue, fmt.Errorf("semantic runtime Brain bootstrap sidecar is not exact canonical JSON")
	}
	value, err := artifact.verifyFor(frozen)
	if err != nil {
		return bootstrapValue, err
	}
	return value, nil
}

func decodeSemanticRuntimeActivation(
	raw []byte,
	frozen BrainFingerprint,
) (activationValue cognitionpolicy.ProviderProcessActivation, err error) {
	var artifact runtimeProviderActivationEvidenceArtifact
	if err := decodeStrictJSON(raw, &artifact, "semantic runtime provider activation sidecar"); err != nil {
		return activationValue, err
	}
	canonical, err := encodeRuntimeProviderActivationEvidenceArtifact(artifact)
	if err != nil || !bytes.Equal(raw, canonical) {
		return activationValue, fmt.Errorf("semantic runtime provider activation sidecar is not exact canonical JSON")
	}
	value, err := artifact.verifyFor(frozen)
	if err != nil {
		return activationValue, err
	}
	return value, nil
}

func addSemanticRuntimeContent(
	supplement *semanticReplaySupplement,
	kind string,
	id string,
	raw []byte,
) error {
	content, chunked, blobs, err := cognitionreplay.NewPublicProjectionContent(
		id, "application/json", raw,
	)
	if err != nil {
		return err
	}
	return supplement.add(kind, id, content, chunked, blobs)
}
