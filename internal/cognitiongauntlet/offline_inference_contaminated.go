package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/labyrinth"
)

// loadContaminatedInferenceGrant is reachable only for the explicitly labeled
// oracle-evidence ceiling. Ordinary inference configurations cannot carry this
// credentialed authority.
func loadContaminatedInferenceGrant(
	config inferenceProcessConfig,
	bundle PublicInferenceBundle,
) (*labyrinth.Oracle, error) {
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
	oracle := private.Oracle
	return &oracle, nil
}
