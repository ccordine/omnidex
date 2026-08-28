package llm

import (
	"fmt"
	"testing"
	"time"
)

func TestExactProviderResponseTimestampAuthority(t *testing.T) {
	t.Parallel()
	for _, timestamp := range []string{
		"2026-08-09T22:00:00Z",
		"2026-08-09T22:00:00.123456789Z",
	} {
		if _, err := DecodeExactPreparedResponse(200, exactTimestampResponse(timestamp)); err != nil {
			t.Fatalf("canonical timestamp %q: %v", timestamp, err)
		}
	}
	for _, timestamp := range []string{
		"0000-08-09T22:00:00Z",
		"2026-08-09T24:00:00Z",
		"2026-08-09T22:00:60Z",
		"2026-02-30T22:00:00Z",
		"2026-08-09T22:00:00.100Z",
	} {
		if _, err := DecodeExactPreparedResponse(200, exactTimestampResponse(timestamp)); err == nil {
			t.Fatalf("noncanonical provider timestamp %q was accepted", timestamp)
		}
	}
}

func TestProviderIdentityObservationRejectsYearZero(t *testing.T) {
	t.Parallel()
	expected := providerIdentityTestExpectation()
	attestation, err := NewProviderIdentityAttestation(
		expected, "test:/version", "test:/installed", "test:/runner",
	)
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := DeriveProviderIdentityObservationChallenge("year-zero", expected)
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewObservedProviderIdentity(
		time.Date(0, time.August, 9, 22, 0, 0, 0, time.UTC),
		attestation, providerIdentityTestEvidence(t, expected), challenge,
	)
	if err == nil {
		t.Fatal("provider observation accepted a year PostgreSQL cannot preserve")
	}
}

func exactTimestampResponse(timestamp string) []byte {
	return []byte(fmt.Sprintf(
		`{"model":"model:test","created_at":%q,"response":"semantic leaf","done":true,`+
			`"done_reason":"stop","total_duration":100,"load_duration":10,`+
			`"prompt_eval_count":12,"prompt_eval_duration":20,"eval_count":3,`+
			`"eval_duration":30}`,
		timestamp,
	))
}
