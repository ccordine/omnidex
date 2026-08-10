package cognitionpolicy

import (
	"bytes"
	"testing"
)

func TestProviderGenerationEvidencePreservesMaximumBoundedResponse(t *testing.T) {
	projection := policyTestProjection(t, "maximum provider evidence")
	snapshot, _ := policyTestSnapshot(t, projection)
	rendered, err := Render(snapshot, projection, policyTestBrain())
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := newCallAttempt(snapshot, policyTestAttestedBrain(), rendered)
	if err != nil {
		t.Fatal(err)
	}
	generation := policyTestPreparedGeneration(
		attempt, string(bytes.Repeat([]byte{'x'}, MaxModelResponseEvidenceBytes-(2*1024))),
	)
	evidence, err := NewProviderGenerationEvidence(attempt.ID, generation)
	if err != nil {
		t.Fatalf("maximum bounded response evidence: %v", err)
	}
	if evidence.Ref.Bytes > MaxProviderGenerationEvidenceBytes {
		t.Fatalf("evidence bytes=%d maximum=%d", evidence.Ref.Bytes, MaxProviderGenerationEvidenceBytes)
	}
	decoded, err := decodeProviderGenerationEvidence(evidence.Generation)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Content != generation.Content {
		t.Fatal("maximum model response changed in provider evidence")
	}
}

func TestProviderGenerationEvidencePreservesInvalidUTF8AndRejectsNoncanonicalWire(t *testing.T) {
	projection := policyTestProjection(t, "opaque provider evidence")
	snapshot, _ := policyTestSnapshot(t, projection)
	rendered, err := Render(snapshot, projection, policyTestBrain())
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := newCallAttempt(snapshot, policyTestAttestedBrain(), rendered)
	if err != nil {
		t.Fatal(err)
	}
	generation := policyTestPreparedGeneration(attempt, string([]byte{0xff, 0xfe, 'x'}))
	evidence, err := NewProviderGenerationEvidence(attempt.ID, generation)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeProviderGenerationEvidence(evidence.Generation)
	if err != nil || !bytes.Equal([]byte(decoded.Content), []byte(generation.Content)) {
		t.Fatalf("opaque invalid UTF-8 changed: %x / %v", []byte(decoded.Content), err)
	}

	noncanonical := append([]byte(" \n"), evidence.Generation...)
	evidence.Generation = noncanonical
	evidence.Ref = providerGenerationEvidenceRef(attempt.ID, noncanonical)
	if err := evidence.Validate(); err == nil {
		t.Fatal("noncanonical provider generation wire validated")
	}
}
