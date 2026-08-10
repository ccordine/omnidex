package cognitiongauntlet

import "fmt"

func SealOfflineScaleReceipt(
	path string,
	receipt OfflineScaleReceipt,
	registration OfflineScalePreregistration,
) error {
	if err := receipt.Validate(registration); err != nil {
		return err
	}
	return sealScenarioArtifact(path, receipt, "offline Scale receipt")
}

func LoadOfflineScaleReceipt(
	path string,
	registration OfflineScalePreregistration,
) (OfflineScaleReceipt, error) {
	var receipt OfflineScaleReceipt
	if err := loadStrictJSONFile(path, &receipt, "offline Scale receipt"); err != nil {
		return OfflineScaleReceipt{}, err
	}
	if err := receipt.Validate(registration); err != nil {
		return OfflineScaleReceipt{}, err
	}
	return receipt, nil
}

func LoadVerifiedOfflineScaleReceipt(
	config OfflineScaleConfig,
) (VerifiedOfflineScaleReceipt, error) {
	if err := config.Validate(); err != nil {
		return VerifiedOfflineScaleReceipt{}, err
	}
	registration, err := LoadOfflineScalePreregistration(config.Paths().Preregistration)
	if err != nil {
		return VerifiedOfflineScaleReceipt{}, err
	}
	receipt, err := LoadOfflineScaleReceipt(config.Paths().Receipt, registration)
	if err != nil {
		return VerifiedOfflineScaleReceipt{}, err
	}
	artifacts, err := loadAllOfflineScaleArtifacts(config, registration)
	if err != nil {
		return VerifiedOfflineScaleReceipt{}, err
	}
	rebuilt, err := buildOfflineScaleReceipt(
		registration, artifacts, receipt.LastInferenceExitedAt,
		receipt.FirstEvaluatorStartedAt,
	)
	if err != nil {
		return VerifiedOfflineScaleReceipt{}, err
	}
	if !equalOfflineScaleReceipt(receipt, rebuilt) {
		return VerifiedOfflineScaleReceipt{}, fmt.Errorf(
			"offline Scale receipt differs from exact sealed artifacts",
		)
	}
	return VerifiedOfflineScaleReceipt{receipt: receipt}, nil
}
