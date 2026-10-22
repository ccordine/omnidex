package llm

const MaxOwnedPreparedGenerationBytes = MaxExactPreparedModelContentBytes +
	MaxExactPreparedProviderResponseBytes + maxExactPreparedProviderBoundaryBytes

// OwnBoundedPreparedGeneration checks every provider-owned slice before the
// policy allocates a private copy. Metadata counters are never trusted as a
// substitute for measuring the actual returned slices.
func OwnBoundedPreparedGeneration(
	generation PreparedGeneration,
) (PreparedGeneration, error) {

	total := len(generation.Content)

	total += len(generation.ProviderResponseCapture)
	generation.ProviderResponseCapture = append(
		[]byte(nil), generation.ProviderResponseCapture...,
	)
	return generation, nil
}
