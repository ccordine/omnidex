package cognitionpolicy

import (
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/llm"
)

func TestProviderIdentityFailureProofBoundsNestedMetadataAndAggregate(t *testing.T) {
	t.Parallel()
	proof := providerIdentityFailureProof{
		Attestation: llm.ProviderIdentityAttestation{BackendEvidence: strings.Repeat("a", 40*1024)},
		Observation: llm.ProviderIdentityObservation{
			Evidence: llm.ProviderIdentityEvidenceRef{ID: strings.Repeat("b", 25*1024)},
		},
	}
	if providerIdentityFailureProofBounded(proof) {
		t.Fatal("aggregate nested provider metadata above the registered cap was accepted")
	}
	proof.Observation.Evidence.ID = strings.Repeat("b", 24*1024-1)
	if !providerIdentityFailureProofBounded(proof) {
		t.Fatal("provider metadata at the registered aggregate cap was rejected")
	}
}

func TestProviderMetadataUnboundedIsNotADurableFailureLabel(t *testing.T) {
	t.Parallel()
	brain := policyTestAttestedBrain()
	request, err := BootstrapProviderIdentityRequest(brain.Ref)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := policyTestObservedProviderIdentity(
		time.Date(2026, 8, 11, 4, 0, 0, 0, time.UTC), brain.Attestation,
		request.ChallengeSHA256,
	)
	if err != nil {
		t.Fatal(err)
	}
	receipt := BrainBootstrapFailureReceipt{
		Schema: BrainBootstrapFailureSchemaV1, Brain: brain.Ref,
		ChallengeSHA256: request.ChallengeSHA256,
		Code:            ProviderIdentityFailureCode("provider_metadata_unbounded"),
		Evidence:        observed.Evidence.Ref,
	}
	receipt.ID = brainBootstrapFailureID(receipt)
	failure := BrainBootstrapFailure{Receipt: receipt, IdentityEvidence: observed.Evidence}
	if err := failure.Validate(); err == nil {
		t.Fatal("ordinary successful evidence was relabeled as unbounded provider metadata")
	}

	oversized := observed
	oversized.Attestation.BackendEvidence = strings.Repeat("x", providerIdentityFailureMetadataBytes+1)
	if code, codeErr := providerIdentityFailureCodeForObserved(brain.Ref, request, oversized); codeErr == nil || code != "" {
		t.Fatalf("unrecordable adapter metadata code=%q error=%v", code, codeErr)
	}
}
