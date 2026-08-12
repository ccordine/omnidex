package cognitiongauntlet

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

func TestSemanticRuntimeSidecarsBindExactQueueProviderAuthorities(t *testing.T) {
	fixture := semanticRuntimeSidecarFixture(t)
	var supplement semanticReplaySupplement
	if err := addSemanticRuntimeSidecars(
		fixture.sidecars, fixture.frozen, fixture.inventory, fixture.identities, &supplement,
	); err != nil {
		t.Fatal(err)
	}
	if len(supplement.sidecars) != 2 {
		t.Fatalf("runtime sidecars=%d want 2", len(supplement.sidecars))
	}
	changed := fixture
	changed.inventory.initialObservation.ID += "-changed"
	if err := addSemanticRuntimeSidecars(
		changed.sidecars, changed.frozen, changed.inventory, changed.identities,
		&semanticReplaySupplement{},
	); err == nil {
		t.Fatal("runtime activation sidecar accepted another queue receipt")
	}
	changed = fixture
	changed.sidecars.RuntimeProviderActivationEvidence = append(
		[]byte(nil), changed.sidecars.RuntimeProviderActivationEvidence...,
	)
	changed.sidecars.RuntimeProviderActivationEvidence[1] ^= 1
	if err := addSemanticRuntimeSidecars(
		changed.sidecars, changed.frozen, changed.inventory, changed.identities,
		&semanticReplaySupplement{},
	); err == nil {
		t.Fatal("mutated runtime activation sidecar was accepted")
	}
}

func TestSemanticRuntimeSidecarsRejectSwapAndTruncation(t *testing.T) {
	fixture := semanticRuntimeSidecarFixture(t)
	for name, mutate := range map[string]func(*ProductionSemanticReplaySidecars){
		"swap": func(value *ProductionSemanticReplaySidecars) {
			value.RuntimeBrainBootstrapEvidence, value.RuntimeProviderActivationEvidence =
				value.RuntimeProviderActivationEvidence, value.RuntimeBrainBootstrapEvidence
		},
		"truncate bootstrap": func(value *ProductionSemanticReplaySidecars) {
			value.RuntimeBrainBootstrapEvidence = value.RuntimeBrainBootstrapEvidence[:len(value.RuntimeBrainBootstrapEvidence)-1]
		},
		"truncate activation": func(value *ProductionSemanticReplaySidecars) {
			value.RuntimeProviderActivationEvidence = value.RuntimeProviderActivationEvidence[:len(value.RuntimeProviderActivationEvidence)-1]
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := fixture.sidecars
			mutate(&changed)
			if err := addSemanticRuntimeSidecars(
				changed, fixture.frozen, fixture.inventory, fixture.identities,
				&semanticReplaySupplement{},
			); err == nil {
				t.Fatal("changed runtime provider sidecars were accepted")
			}
		})
	}
}

type semanticRuntimeSidecarTestFixture struct {
	frozen     BrainFingerprint
	bootstrap  cognitionpolicy.BrainBootstrap
	activation cognitionpolicy.ProviderProcessActivation
	sidecars   ProductionSemanticReplaySidecars
	inventory  semanticReplayEvidenceInventory
	identities map[string]llm.ProviderIdentityEvidence
}

func semanticRuntimeSidecarFixture(t *testing.T) semanticRuntimeSidecarTestFixture {
	t.Helper()
	frozen := mustRatGeneration(t).Fixed.Brain
	brain, err := frozen.attestedBrain()
	if err != nil {
		t.Fatal(err)
	}
	bootstrapEvidence, err := witnessProviderIdentityEvidence(brain.Attestation)
	if err != nil {
		t.Fatal(err)
	}
	bootstrap, err := cognitionpolicy.NewBrainBootstrap(brain, bootstrapEvidence)
	if err != nil {
		t.Fatal(err)
	}
	episode, err := cognition.NewEpisodeRef(
		cognition.EpisodeID("episode-" + strings.Repeat("a", 64)),
	)
	if err != nil {
		t.Fatal(err)
	}
	actor := cognition.AttemptRef{
		JobID: 1, Generation: 1, StepID: 1, Attempt: 1, WorkerID: "worker-one",
	}
	client := &witnessPolicyClient{model: brain.Ref.Model}
	outcome, err := cognitionpolicy.ObserveProviderProcess(
		t.Context(), client, brain, episode, actor,
		cognitionpolicy.ProviderProcessEpisodeInvocation,
	)
	if err != nil {
		t.Fatal(err)
	}
	activation, err := outcome.RequireSuccess(brain)
	if err != nil {
		t.Fatal(err)
	}
	bootstrapArtifact, _, err := prepareRuntimeBrainBootstrapEvidence(
		"episode.json", bootstrap, frozen,
	)
	if err != nil {
		t.Fatal(err)
	}
	bootstrapRaw, err := encodeRuntimeBrainBootstrapEvidenceArtifact(bootstrapArtifact)
	if err != nil {
		t.Fatal(err)
	}
	activationArtifact, _, err := prepareRuntimeProviderActivationEvidence(
		"episode.json", activation, frozen,
	)
	if err != nil {
		t.Fatal(err)
	}
	activationRaw, err := encodeRuntimeProviderActivationEvidenceArtifact(activationArtifact)
	if err != nil {
		t.Fatal(err)
	}
	identities := map[string]llm.ProviderIdentityEvidence{
		bootstrapEvidence.Ref.ID:           bootstrapEvidence,
		activation.IdentityEvidence.Ref.ID: activation.IdentityEvidence,
	}
	return semanticRuntimeSidecarTestFixture{
		frozen: frozen, bootstrap: bootstrap, activation: activation,
		sidecars: ProductionSemanticReplaySidecars{
			RuntimeBrainBootstrapEvidence:     bytes.Clone(bootstrapRaw),
			RuntimeProviderActivationEvidence: bytes.Clone(activationRaw),
		},
		inventory: semanticReplayEvidenceInventory{
			initialBootstrap: queue.CognitionBrainBootstrapTrace{
				Source: queue.CognitionBrainBootstrapEpisodeStart, EpisodeID: episode.ID,
				Actor: actor, Brain: brain, Evidence: bootstrapEvidence.Ref,
			},
			initialObservation: activation.Receipt,
		},
		identities: identities,
	}
}
