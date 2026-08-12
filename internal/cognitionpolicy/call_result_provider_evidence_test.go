package cognitionpolicy

import (
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/llm"
)

func TestProviderEvidenceFailureRequiresExactRawOutcomeReference(t *testing.T) {
	t.Parallel()
	attempt := policyTestCallAttempt(t)
	generation := llm.PreparedGeneration{
		Schema:                     llm.PreparedGenerationSchemaV1,
		ProviderRequestDisposition: llm.ProviderRequestNotDispatched,
	}
	evidence, err := NewProviderGenerationEvidence(attempt.ID, generation)
	if err != nil {
		t.Fatal(err)
	}
	valid := untrustedProviderFailedCallResult(
		attempt, llm.ProviderRequestNotDispatched, llm.ProviderIdentityEvidenceRef{},
		evidence.Ref, ProviderResponseCaptureEvidenceRef{},
		CallFailureProviderEvidence, errors.New("invalid provider evidence"),
	)
	if err := valid.Validate(attempt); err != nil {
		t.Fatalf("bounded provider evidence failure: %v", err)
	}

	missing := valid
	missing.ProviderGenerationEvidence = ProviderGenerationEvidenceRef{}
	if err := missing.Validate(attempt); err == nil {
		t.Fatal("provider evidence failure without its raw outcome reference was accepted")
	}
	unknownDisposition := valid
	unknownDisposition.ProviderRequestDisposition = llm.ProviderRequestDisposition("forged")
	if err := unknownDisposition.Validate(attempt); err == nil {
		t.Fatal("provider evidence failure accepted an unknown request disposition")
	}
}
