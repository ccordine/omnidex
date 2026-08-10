package cognitiongauntlet

import (
	"context"
	"fmt"

	buildversion "github.com/gryph/omnidex/internal/version"
)

func RunOfflineScaleGeneratorProcess(ctx context.Context, configPath string) error {
	if ctx == nil {
		return fmt.Errorf("offline Scale generator context is nil")
	}
	var config scaleGeneratorProcessConfig
	if err := loadStrictJSONFile(
		configPath, &config, "offline Scale generator process configuration",
	); err != nil {
		return err
	}
	if err := config.Validate(); err != nil {
		return err
	}
	if err := validateCurrentProcessIdentity(
		config.ExecutableSHA256, config.OmnidexCommit, config.SourceSHA256,
		buildversion.Commit, buildversion.SourceSHA256,
	); err != nil {
		return err
	}
	family, err := generateOfflineScaleFamily(config.Registration)
	if err != nil {
		return err
	}
	artifacts := make([]offlineScaleGeneratedArtifacts, len(config.Outputs))
	for index, output := range config.Outputs {
		generated, err := family.scenario(config.Registration, output.Case)
		if err != nil {
			return fmt.Errorf("generate Scale case %d: %w", index+1, err)
		}
		paired, err := generated.pairedAuthority(
			SurfaceSymbolic, config.Registration.Fixed.RatGeneration,
			output.Case.Repetition, config.Registration.Fixed.RuntimeFingerprint,
		)
		if err != nil {
			return err
		}
		bundle, err := newScenarioPublicInferenceBundle(
			generated.scenario, paired, VariantFullCognition,
		)
		if err != nil {
			return err
		}
		hostRaw, err := generated.scenario.MarshalPrivateJSON()
		if err != nil {
			return err
		}
		private, err := newPrivateScaleEvaluationFixture(
			config.Registration, output.Case, family.descriptor, generated, paired,
		)
		if err != nil {
			return err
		}
		artifacts[index] = offlineScaleGeneratedArtifacts{
			output: output, bundle: bundle, hostRaw: hostRaw, private: private,
		}
	}
	for index, artifact := range artifacts {
		if err := artifact.seal(config.PrivateOracleCredential); err != nil {
			return fmt.Errorf("seal Scale case %d: %w", index+1, err)
		}
	}
	return nil
}

type offlineScaleGeneratedArtifacts struct {
	output  scaleProcessOutput
	bundle  PublicInferenceBundle
	hostRaw []byte
	private privateScaleEvaluationFixture
}

func (artifact offlineScaleGeneratedArtifacts) seal(credential string) error {
	if err := SealPublicInferenceBundle(artifact.output.PublicBundlePath, artifact.bundle); err != nil {
		return err
	}
	if err := writeExclusiveAtomic(
		artifact.output.HostScenarioPath, append(artifact.hostRaw, '\n'),
	); err != nil {
		return fmt.Errorf("seal private Scale host scenario: %w", err)
	}
	return sealPrivateScaleFixture(
		artifact.output.PrivateOraclePath, artifact.private, credential,
	)
}
