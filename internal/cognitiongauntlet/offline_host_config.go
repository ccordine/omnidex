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
	return newHostProcessConfigForExecution(
		config.executionAuthority(), database, bundle, hostScenarioPath,
		config.Paths().PublicBundle, readyPath, environmentToken, executableSHA256,
	)
}

func newHostProcessConfigForExecution(
	authority offlineExecutionAuthority,
	database *offlinePromotionDatabase,
	bundle PublicInferenceBundle,
	hostScenarioPath string,
	publicBundlePath string,
	readyPath string,
	environmentToken string,
	executableSHA256 string,
) hostProcessConfig {
	return hostProcessConfig{
		Schema: hostProcessConfigSchemaV1, DatabaseURL: database.hostURL,
		DatabaseSchema: database.schema, HostSchema: database.hostSchema,
		ExpectedRole: database.hostRole, Scenario: bundle.Authority.Scenario,
		HostScenarioPath: hostScenarioPath, PublicBundlePath: publicBundlePath,
		ReadyPath: readyPath, EnvironmentToken: environmentToken,
		ExecutableSHA256: executableSHA256,
		SourceSHA256:     authority.RatGeneration.Runtime.SourceSHA256,
		OmnidexCommit:    authority.OmnidexCommit,
	}
}
