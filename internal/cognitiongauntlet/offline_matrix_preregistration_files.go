package cognitiongauntlet

func SealOfflineMatrixPreregistration(
	path string,
	registration OfflineMatrixPreregistration,
) error {
	if err := registration.Validate(); err != nil {
		return err
	}
	return sealScenarioArtifact(path, registration, "offline cognition matrix preregistration")
}

func LoadOfflineMatrixPreregistration(
	path string,
) (OfflineMatrixPreregistration, error) {
	var registration OfflineMatrixPreregistration
	if err := loadScenarioArtifact(
		path, &registration, "offline cognition matrix preregistration",
	); err != nil {
		return OfflineMatrixPreregistration{}, err
	}
	if err := registration.Validate(); err != nil {
		return OfflineMatrixPreregistration{}, err
	}
	return registration, nil
}
