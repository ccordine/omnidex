package cognitiongauntlet

func SealOfflineScalePreregistration(
	path string,
	registration OfflineScalePreregistration,
) error {
	if err := registration.Validate(); err != nil {
		return err
	}
	return sealScenarioArtifact(path, registration, "offline Scale preregistration")
}

func LoadOfflineScalePreregistration(path string) (OfflineScalePreregistration, error) {
	var registration OfflineScalePreregistration
	if err := loadStrictJSONFile(path, &registration, "offline Scale preregistration"); err != nil {
		return OfflineScalePreregistration{}, err
	}
	if err := registration.Validate(); err != nil {
		return OfflineScalePreregistration{}, err
	}
	return registration, nil
}
