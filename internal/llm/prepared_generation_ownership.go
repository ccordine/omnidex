package llm

import "fmt"

const MaxOwnedPreparedGenerationBytes = MaxExactPreparedModelContentBytes +
	MaxExactPreparedProviderResponseBytes + maxExactPreparedProviderBoundaryBytes

// OwnBoundedPreparedGeneration checks every provider-owned slice before the
// policy allocates a private copy. Metadata counters are never trusted as a
// substitute for measuring the actual returned slices.
func OwnBoundedPreparedGeneration(
	generation PreparedGeneration,
) (PreparedGeneration, error) {
	if len(generation.Content) > MaxExactPreparedModelContentBytes ||
		len(generation.ProviderResponseCapture) >
			MaxExactPreparedProviderResponseBytes+maxExactPreparedProviderBoundaryBytes {
		return PreparedGeneration{}, fmt.Errorf("prepared generation response exceeds its ownership bound")
	}
	total := len(generation.Content)
	if len(generation.ProviderResponseCapture) > MaxOwnedPreparedGenerationBytes-total {
		return PreparedGeneration{}, fmt.Errorf("prepared generation aggregate exceeds its ownership bound")
	}
	total += len(generation.ProviderResponseCapture)
	generation.ProviderResponseCapture = append(
		[]byte(nil), generation.ProviderResponseCapture...,
	)
	return generation, nil
}
