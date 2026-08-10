package cognitiongauntlet

func SealOfflineTransferPreregistration(
	path string,
	registration OfflineTransferPreregistration,
) error {
	if err := registration.Validate(); err != nil {
		return err
	}
	return sealScenarioArtifact(path, registration, "offline Transfer preregistration")
}

func LoadOfflineTransferPreregistration(
	path string,
) (OfflineTransferPreregistration, error) {
	var registration OfflineTransferPreregistration
	if err := loadStrictJSONFile(
		path, &registration, "offline Transfer preregistration",
	); err != nil {
		return OfflineTransferPreregistration{}, err
	}
	if err := registration.Validate(); err != nil {
		return OfflineTransferPreregistration{}, err
	}
	return registration, nil
}
