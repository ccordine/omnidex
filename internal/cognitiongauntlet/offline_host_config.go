package cognitiongauntlet

func newHostProcessConfig(
	config OfflinePromotionConfig,
	database *offlinePromotionDatabase,
	bundle PublicInferenceBundle,
	hostScenarioPath string,
	readyPath string,
	environmentToken string,
	executableSHA256 string,
) hostProcessConfig {
	return hostProcessConfig{
		Schema: hostProcessConfigSchemaV1, DatabaseURL: database.hostURL,
		DatabaseSchema: database.schema, HostSchema: database.hostSchema,
		ExpectedRole: database.hostRole, Scenario: bundle.Authority.Scenario,
		HostScenarioPath: hostScenarioPath, PublicBundlePath: config.Paths().PublicBundle,
		ReadyPath: readyPath, EnvironmentToken: environmentToken,
		ExecutableSHA256: executableSHA256,
		SourceSHA256:     config.RatGeneration.Runtime.SourceSHA256,
		OmnidexCommit:    config.OmnidexCommit,
	}
}
