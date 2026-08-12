package cognitionpolicy

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/exactjson"
)

func TestProviderGenerationWireValidatesNestedTimeBeforeIncompleteReturn(t *testing.T) {
	evidence := policyTestProviderGenerationEvidence(t)
	for _, mutate := range []func(*providerGenerationEvidenceWire){
		func(wire *providerGenerationEvidenceWire) {
			wire.Schema = providerGenerationOverflowWitness()
			wire.ProviderObservation.ObservedMonth = 13
		},
		func(wire *providerGenerationEvidenceWire) {
			wire.ProviderObservation.Schema = providerGenerationOverflowWitness()
			wire.ProviderObservation.ObservedMonth = 13
		},
	} {
		forged := mutateProviderGenerationWire(t, evidence, mutate)
		if err := forged.Validate(); err == nil {
			t.Fatal("invalid nested observation time hid behind an incomplete wire field")
		}
	}
}

func TestProviderGenerationWireIncompleteAndOpaqueErrorsAreTotal(t *testing.T) {
	evidence := policyTestProviderGenerationEvidence(t)
	for _, present := range []bool{false, true} {
		forged := mutateProviderGenerationWire(t, evidence, func(wire *providerGenerationEvidenceWire) {
			wire.ProviderErrorPresent = present
			wire.ProviderError = providerGenerationOverflowWitness()
		})
		if err := forged.Validate(); err != nil {
			t.Fatalf("incomplete provider error present=%t: %v", present, err)
		}
	}
	forged := mutateProviderGenerationWire(t, evidence, func(wire *providerGenerationEvidenceWire) {
		wire.ProviderErrorPresent = true
		wire.ProviderError = newProviderGenerationWireString(
			string([]byte{0xff}), maxProviderGenerationMetadataCaptureBytes,
		)
	})
	if err := forged.Validate(); err != nil {
		t.Fatalf("opaque provider error: %v", err)
	}
}

func policyTestProviderGenerationEvidence(t *testing.T) ProviderGenerationEvidence {
	t.Helper()
	projection := policyTestProjection(t, "provider wire totality")
	snapshot, _ := policyTestSnapshot(t, projection)
	brain := policyTestAttestedBrain()
	rendered, err := Render(snapshot, projection, policyTestBrain())
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := newCallAttempt(
		snapshot, brain, policyTestProviderProcessActivation(snapshot, brain), rendered,
	)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := NewProviderGenerationEvidence(
		attempt.ID, policyTestPreparedGeneration(attempt, `{}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	return evidence
}

func providerGenerationOverflowWitness() providerGenerationWireBytes {
	return newProviderGenerationWireString(
		strings.Repeat("x", maxProviderGenerationMetadataCaptureBytes+1),
		maxProviderGenerationMetadataCaptureBytes,
	)
}

func mutateProviderGenerationWire(
	t *testing.T,
	evidence ProviderGenerationEvidence,
	mutate func(*providerGenerationEvidenceWire),
) ProviderGenerationEvidence {
	t.Helper()
	var wire providerGenerationEvidenceWire
	if err := json.Unmarshal(evidence.Generation, &wire); err != nil {
		t.Fatal(err)
	}
	mutate(&wire)
	raw, err := exactjson.Canonical(wire)
	if err != nil {
		t.Fatal(err)
	}
	evidence.Generation = raw
	evidence.Ref = providerGenerationEvidenceRef(evidence.CallID, raw)
	return evidence
}
