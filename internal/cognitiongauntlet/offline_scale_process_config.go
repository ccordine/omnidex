package cognitiongauntlet

import "path/filepath"

func newScaleGeneratorProcessConfig(
	config OfflineScaleConfig,
	registration OfflineScalePreregistration,
	temporary string,
	credential string,
	executableSHA256 string,
) scaleGeneratorProcessConfig {
	outputs := make([]scaleProcessOutput, len(registration.Cases))
	for index, current := range registration.Cases {
		paths := config.runPaths(current)
		outputs[index] = scaleProcessOutput{
			Case: current, PublicBundlePath: paths.PublicBundle,
			HostScenarioPath:  filepath.Join(temporary, "host-"+current.ID+".json"),
			PrivateOraclePath: paths.PrivateOracle,
		}
	}
	return scaleGeneratorProcessConfig{
		Schema: scaleGeneratorProcessSchemaV1, Registration: registration, Outputs: outputs,
		PrivateOracleCredential: credential, ExecutableSHA256: executableSHA256,
		SourceSHA256:  config.RatGeneration.Runtime.SourceSHA256,
		OmnidexCommit: config.OmnidexCommit,
	}
}

func newScaleEvaluatorProcessConfig(
	config OfflineScaleConfig,
	current OfflineScaleCase,
	credential string,
	executableSHA256 string,
) scaleEvaluatorProcessConfig {
	paths := config.runPaths(current)
	return scaleEvaluatorProcessConfig{
		Schema: scaleEvaluatorProcessSchemaV1, PrivateOraclePath: paths.PrivateOracle,
		PrivateOracleCredential: credential, PublicBundlePath: paths.PublicBundle,
		EpisodePath: paths.Episode, EvaluationPath: paths.Evaluation,
		ScaleEvidencePath: config.scaleEvidencePath(current),
		ExecutableSHA256:  executableSHA256,
		SourceSHA256:      config.RatGeneration.Runtime.SourceSHA256,
		OmnidexCommit:     config.OmnidexCommit,
	}
}
