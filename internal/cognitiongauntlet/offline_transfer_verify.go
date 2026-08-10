package cognitiongauntlet

import "fmt"

func SealOfflineTransferReceipt(
	path string,
	receipt OfflineTransferReceipt,
	registration OfflineTransferPreregistration,
) error {
	if err := receipt.Validate(registration); err != nil {
		return err
	}
	return sealScenarioArtifact(path, receipt, "offline Transfer receipt")
}

func LoadOfflineTransferReceipt(
	path string,
	registration OfflineTransferPreregistration,
) (OfflineTransferReceipt, error) {
	var receipt OfflineTransferReceipt
	if err := loadStrictJSONFile(path, &receipt, "offline Transfer receipt"); err != nil {
		return OfflineTransferReceipt{}, err
	}
	if err := receipt.Validate(registration); err != nil {
		return OfflineTransferReceipt{}, err
	}
	return receipt, nil
}

func LoadVerifiedOfflineTransferReceipt(
	config OfflineTransferConfig,
) (VerifiedOfflineTransferReceipt, error) {
	if err := config.Validate(); err != nil {
		return VerifiedOfflineTransferReceipt{}, err
	}
	registration, err := LoadOfflineTransferPreregistration(config.Paths().Preregistration)
	if err != nil {
		return VerifiedOfflineTransferReceipt{}, err
	}
	receipt, err := LoadOfflineTransferReceipt(config.Paths().Receipt, registration)
	if err != nil {
		return VerifiedOfflineTransferReceipt{}, err
	}
	artifacts, err := loadAllOfflineTransferArtifacts(config, registration)
	if err != nil {
		return VerifiedOfflineTransferReceipt{}, err
	}
	rebuilt, err := buildOfflineTransferReceipt(registration, artifacts)
	if err != nil {
		return VerifiedOfflineTransferReceipt{}, err
	}
	if !equalOfflineTransferReceipt(receipt, rebuilt) {
		return VerifiedOfflineTransferReceipt{}, fmt.Errorf("offline Transfer receipt differs from exact sealed artifacts")
	}
	return VerifiedOfflineTransferReceipt{receipt: receipt}, nil
}
