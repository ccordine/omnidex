package llm

// Clone takes code ownership of every provider-supplied byte slice before the
// policy validates or journals the exact outcome.
func (generation PreparedGeneration) Clone() PreparedGeneration {
	generation.ProviderResponseCapture = append(
		[]byte(nil), generation.ProviderResponseCapture...,
	)
	generation.ProviderIdentityEvidence = generation.ProviderIdentityEvidence.Clone()
	return generation
}
