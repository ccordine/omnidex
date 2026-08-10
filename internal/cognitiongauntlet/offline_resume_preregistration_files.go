package cognitiongauntlet

func SealOfflineResumePreregistration(
	path string,
	registration OfflineResumePreregistration,
) error {
	if err := registration.Validate(); err != nil {
		return err
	}
	return sealScenarioArtifact(path, registration, "offline Resume preregistration")
}

func LoadOfflineResumePreregistration(
	path string,
) (OfflineResumePreregistration, error) {
	var registration OfflineResumePreregistration
	if err := loadStrictJSONFile(path, &registration, "offline Resume preregistration"); err != nil {
		return OfflineResumePreregistration{}, err
	}
	if err := registration.Validate(); err != nil {
		return OfflineResumePreregistration{}, err
	}
	return registration, nil
}
