package llm

import "fmt"

const MaxOwnedPreparedGenerationBytes = (2 * MaxExactPreparedProviderResponseBytes) + 1 + MaxProviderIdentityEvidenceBytes

// OwnBoundedPreparedGeneration checks every provider-owned slice before the
// policy allocates a private copy. Metadata counters are never trusted as a
// substitute for measuring the actual returned slices.
func OwnBoundedPreparedGeneration(
	generation PreparedGeneration,
) (PreparedGeneration, error) {
	if len(generation.Content) > MaxExactPreparedProviderResponseBytes ||
		len(generation.ProviderResponseCapture) > MaxExactPreparedProviderResponseBytes+1 {
		return PreparedGeneration{}, fmt.Errorf("prepared generation response exceeds its ownership bound")
	}
	if err := validateProviderIdentityOwnershipBounds(
		generation.ProviderIdentityEvidence,
	); err != nil {
		return PreparedGeneration{}, fmt.Errorf("prepared generation identity: %w", err)
	}
	identityBytes := 0
	for index := range generation.ProviderIdentityEvidence.Operations {
		identityBytes += len(generation.ProviderIdentityEvidence.Operations[index].Request)
		identityBytes += len(generation.ProviderIdentityEvidence.Operations[index].ResponseCapture)
	}
	total := len(generation.Content)
	if len(generation.ProviderResponseCapture) > MaxOwnedPreparedGenerationBytes-total {
		return PreparedGeneration{}, fmt.Errorf("prepared generation aggregate exceeds its ownership bound")
	}
	total += len(generation.ProviderResponseCapture)
	if identityBytes > MaxOwnedPreparedGenerationBytes-total {
		return PreparedGeneration{}, fmt.Errorf("prepared generation aggregate exceeds its ownership bound")
	}
	generation.ProviderResponseCapture = append(
		[]byte(nil), generation.ProviderResponseCapture...,
	)
	generation.ProviderIdentityEvidence = generation.ProviderIdentityEvidence.Clone()
	return generation, nil
}
