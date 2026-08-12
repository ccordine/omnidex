package cognitionpolicy

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestProviderGenerationEvidenceOwnsZeroByteIdentityComponents(t *testing.T) {
	projection := policyTestProjection(t, "zero byte identity evidence")
	snapshot, _ := policyTestSnapshot(t, projection)
	rendered, err := Render(snapshot, projection, policyTestBrain())
	if err != nil {
		t.Fatal(err)
	}
	brain := policyTestAttestedBrain()
	attempt, err := newCallAttempt(
		snapshot, brain, policyTestProviderProcessActivation(snapshot, brain), rendered,
	)
	if err != nil {
		t.Fatal(err)
	}
	generation := policyTestPreparedGeneration(attempt, `{}`)
	for index := range generation.ProviderIdentityEvidence.Operations {
		operation := &generation.ProviderIdentityEvidence.Operations[index]
		if len(operation.Request) == 0 {
			operation.Request = []byte{}
		}
		if len(operation.ResponseCapture) == 0 {
			operation.ResponseCapture = []byte{}
		}
	}
	evidence, err := NewProviderGenerationEvidence(attempt.ID, generation)
	if err != nil {
		t.Fatal(err)
	}
	decoded, _, _, complete, err := inspectProviderGenerationOutcomeEvidence(evidence.Generation)
	if err != nil || !complete {
		t.Fatalf("inspect complete=%t error=%v", complete, err)
	}
	if !reflect.DeepEqual(
		decoded.ProviderIdentityEvidence, generation.ProviderIdentityEvidence,
	) {
		t.Fatal("zero-byte identity request or response changed across the opaque wire")
	}
}

func TestProviderGenerationEvidencePreservesMaximumBoundedResponse(t *testing.T) {
	projection := policyTestProjection(t, "maximum provider evidence")
	snapshot, _ := policyTestSnapshot(t, projection)
	rendered, err := Render(snapshot, projection, policyTestBrain())
	if err != nil {
		t.Fatal(err)
	}
	brain := policyTestAttestedBrain()
	attempt, err := newCallAttempt(
		snapshot, brain, policyTestProviderProcessActivation(snapshot, brain), rendered,
	)
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
	brain := policyTestAttestedBrain()
	attempt, err := newCallAttempt(
		snapshot, brain, policyTestProviderProcessActivation(snapshot, brain), rendered,
	)
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

func TestProviderGenerationEvidenceBoundsEveryUntrustedContentEncodingField(t *testing.T) {
	projection := policyTestProjection(t, "bounded provider headers")
	snapshot, _ := policyTestSnapshot(t, projection)
	rendered, err := Render(snapshot, projection, policyTestBrain())
	if err != nil {
		t.Fatal(err)
	}
	brain := policyTestAttestedBrain()
	attempt, err := newCallAttempt(
		snapshot, brain, policyTestProviderProcessActivation(snapshot, brain), rendered,
	)
	if err != nil {
		t.Fatal(err)
	}
	generation := policyTestPreparedGeneration(attempt, `{}`)
	malicious := strings.Repeat("z", maxProviderContentEncodingBase64Bytes+2)
	generation.ProviderContentEncoding.CapturedBase64 = malicious
	for index := range generation.ProviderIdentityEvidence.Operations {
		operation := &generation.ProviderIdentityEvidence.Operations[index]
		operation.ContentEncoding.Schema = malicious
		operation.ContentEncoding.SHA256 = malicious
		operation.ContentEncoding.CapturedBase64 = malicious
	}
	evidence, err := NewProviderGenerationEvidence(attempt.ID, generation)
	if err != nil {
		t.Fatalf("bounded malicious content-encoding evidence: %v", err)
	}
	if evidence.Ref.Bytes > MaxProviderGenerationEvidenceBytes {
		t.Fatalf("evidence bytes=%d maximum=%d", evidence.Ref.Bytes, MaxProviderGenerationEvidenceBytes)
	}
	if _, complete, err := inspectProviderGenerationEvidence(evidence.Generation); err != nil || complete {
		t.Fatalf("malicious header evidence complete=%t error=%v", complete, err)
	}
}
