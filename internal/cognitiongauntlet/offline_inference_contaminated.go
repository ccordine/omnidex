package cognitiongauntlet

import "fmt"

// loadContaminatedInferenceGrant is reachable only for the explicitly labeled
// oracle-evidence ceiling. Ordinary inference configurations cannot carry this
// credentialed authority.
func loadContaminatedInferenceGrant(
	config inferenceProcessConfig,
	bundle PublicInferenceBundle,
) (*ContaminatedEvidencePacket, error) {
	if bundle.Authority.Variant != VariantOracleEvidence {
		if config.ContaminatedOraclePath != "" || config.ContaminatedOracleGrant != "" {
			return nil, fmt.Errorf("non-oracle inference received private evaluator authority")
		}
		return nil, nil
	}
	if config.ContaminatedOraclePath == "" || config.ContaminatedOracleGrant == "" {
		return nil, fmt.Errorf("oracle-evidence inference lacks its explicit contaminated grant")
	}
	private, err := loadPrivateEvaluationFixture(
		config.ContaminatedOraclePath, config.ContaminatedOracleGrant,
	)
	if err != nil {
		return nil, err
	}
	if err := ValidatePublicRunAuthorityProjection(private.Authority, bundle.Authority); err != nil {
		return nil, fmt.Errorf("contaminated oracle grant changed public authority: %w", err)
	}
	generated, err := generateOfflineScenario(private.Scenario)
	if err != nil {
		return nil, err
	}
	packet, err := contaminatedEvidenceFor(generated)
	if err != nil {
		return nil, err
	}
	return &packet, nil
}
