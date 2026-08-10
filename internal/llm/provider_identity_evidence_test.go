package llm

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestProviderIdentityEvidenceRequiresOneOrderedFailureBoundary(t *testing.T) {
	t.Parallel()
	expected := providerIdentityTestExpectation()
	valid := providerIdentityTestEvidence(t, expected)

	firstPending := cloneProviderIdentityOperations(valid.Operations)
	firstPending[0] = providerIdentityPendingOperation(t, firstPending[0])
	if _, err := NewProviderIdentityEvidence(firstPending); err == nil {
		t.Fatal("provider identity evidence began with an undispatched operation")
	}

	middleHole := cloneProviderIdentityOperations(valid.Operations)
	middleHole[2] = providerIdentityPendingOperation(t, middleHole[2])
	if _, err := NewProviderIdentityEvidence(middleHole); err == nil {
		t.Fatal("provider identity evidence accepted a middle undispatched hole")
	}

	continued := cloneProviderIdentityOperations(valid.Operations)
	continued[1] = providerIdentityFailedOperation(t, continued[1], ProviderIdentityHTTPError, 503)
	if _, err := NewProviderIdentityEvidence(continued); err == nil {
		t.Fatal("provider identity evidence continued after its first failure")
	}
}

func TestProviderIdentityEvidenceRejectsInvalidHTTPStatusAndInexactAuthority(t *testing.T) {
	t.Parallel()
	expected := providerIdentityTestExpectation()
	valid := providerIdentityTestEvidence(t, expected)

	for _, disposition := range []ProviderIdentityOperationDisposition{
		ProviderIdentityBodyLimit, ProviderIdentityBodyReadError,
	} {
		operation := valid.Operations[0]
		operation.HTTPStatus = 999
		operation.Disposition = disposition
		operation.ResponseComplete = false
		if disposition == ProviderIdentityBodyLimit {
			operation.ResponseCapture = bytes.Repeat([]byte{'x'}, MaxProviderIdentityComponentBytes+1)
			operation.ResponseBytes = len(operation.ResponseCapture)
			operation.ResponseSHA256 = providerBodySHA256(operation.ResponseCapture)
		}
		if err := operation.Validate(); err == nil {
			t.Fatalf("status 999 accepted for %s", disposition)
		}
	}

	changedVersion := cloneProviderIdentityOperations(valid.Operations)
	changedVersion[0].ResponseCapture = []byte(`{"version":" 0.24.0 "}`)
	changedVersion[0].ResponseBytes = len(changedVersion[0].ResponseCapture)
	changedVersion[0].ResponseSHA256 = providerBodySHA256(changedVersion[0].ResponseCapture)
	evidence, err := NewProviderIdentityEvidence(changedVersion)
	if err != nil {
		t.Fatal(err)
	}
	selection := ProviderIdentitySelection{Model: expected.Model, NativeContextLimit: expected.NativeContextLimit}
	if _, err := DeriveExactProviderIdentityExpectation(evidence, selection); err == nil {
		t.Fatal("whitespace-normalized provider version was accepted")
	}

	changedPreload := cloneProviderIdentityOperations(valid.Operations)
	changedPreload[3].Request = append([]byte(" "), changedPreload[3].Request...)
	changedPreload[3].RequestBytes = len(changedPreload[3].Request)
	changedPreload[3].RequestSHA256 = providerBodySHA256(changedPreload[3].Request)
	evidence, err = NewProviderIdentityEvidence(changedPreload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DeriveExactProviderIdentityExpectation(evidence, selection); err == nil {
		t.Fatal("noncanonical provider preload request was accepted")
	}
}

func TestProviderIdentityFailureEvidenceKeepsEveryPlannedRequestExact(t *testing.T) {
	t.Parallel()
	expected := providerIdentityTestExpectation()
	valid := providerIdentityTestEvidence(t, expected)
	operations := cloneProviderIdentityOperations(valid.Operations)
	operations[2] = providerIdentityFailedOperation(
		t, operations[2], ProviderIdentityHTTPError, 503,
	)
	operations[3] = providerIdentityPendingOperation(t, operations[3])
	operations[4] = providerIdentityPendingOperation(t, operations[4])
	selection := ProviderIdentitySelection{
		Model: expected.Model, NativeContextLimit: expected.NativeContextLimit,
	}
	for name, mutate := range map[string]func([]ProviderIdentityOperationEvidence){
		"empty GET": func(values []ProviderIdentityOperationEvidence) {
			values[0].Request = []byte("unexpected")
			values[0].RequestBytes = len(values[0].Request)
			values[0].RequestSHA256 = providerBodySHA256(values[0].Request)
		},
		"show": func(values []ProviderIdentityOperationEvidence) {
			values[2].Request = []byte(`{"model":"other","verbose":false}`)
			values[2].RequestBytes = len(values[2].Request)
			values[2].RequestSHA256 = providerBodySHA256(values[2].Request)
		},
		"pending preload": func(values []ProviderIdentityOperationEvidence) {
			values[3].Request = []byte(`{"model":"other"}`)
			values[3].RequestBytes = len(values[3].Request)
			values[3].RequestSHA256 = providerBodySHA256(values[3].Request)
		},
	} {
		changed := cloneProviderIdentityOperations(operations)
		mutate(changed)
		evidence, err := NewProviderIdentityEvidence(changed)
		if err != nil {
			t.Fatalf("construct self-consistent %s evidence: %v", name, err)
		}
		if err := evidence.ValidateRequests(selection); err == nil {
			t.Fatalf("changed %s request was accepted", name)
		}
	}
}

func providerIdentityPendingOperation(
	t *testing.T,
	operation ProviderIdentityOperationEvidence,
) ProviderIdentityOperationEvidence {
	t.Helper()
	value, err := NewProviderIdentityOperationEvidence(
		operation.Operation, operation.Method, operation.Endpoint, false,
		operation.Request, 0, ProviderIdentityNotDispatched, false, 0, "", false, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func providerIdentityFailedOperation(
	t *testing.T,
	operation ProviderIdentityOperationEvidence,
	disposition ProviderIdentityOperationDisposition,
	status int,
) ProviderIdentityOperationEvidence {
	t.Helper()
	value, err := NewProviderIdentityOperationEvidence(
		operation.Operation, operation.Method, operation.Endpoint, true,
		operation.Request, status, disposition, true, 0, "", false,
		[]byte(`{"error":"failed"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestProviderIdentityObservationProjectsEveryEvidenceField(t *testing.T) {
	t.Parallel()
	expected := providerIdentityTestExpectation()
	evidence := providerIdentityTestEvidence(t, expected)
	attestation, err := NewProviderIdentityAttestation(
		expected, "test:version", "test:installed", "test:runner",
	)
	if err != nil {
		t.Fatal(err)
	}
	challenge := strings.Repeat("b", 64)
	observed, err := NewObservedProviderIdentity(
		time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC), attestation, evidence, challenge,
	)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*ProviderIdentityObservation){
		"tokenizer request": func(value *ProviderIdentityObservation) { value.TokenizerRequestSHA256 = strings.Repeat("c", 64) },
		"tokenizer body":    func(value *ProviderIdentityObservation) { value.TokenizerBodySHA256 = strings.Repeat("d", 64) },
		"evidence":          func(value *ProviderIdentityObservation) { value.Evidence.SHA256 = strings.Repeat("e", 64) },
	} {
		changed := observed.Observation
		mutate(&changed)
		if err := changed.ValidateEvidence(evidence); err == nil {
			t.Fatalf("changed %s evidence projection was accepted", name)
		}
	}
}
