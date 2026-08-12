package cognitiongauntlet

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func PrepareOfflineResumeConfig(ctx context.Context, requestPath, configPath string) error {
	if ctx == nil || requestPath == "" || configPath == "" || requestPath == configPath ||
		filepath.Clean(requestPath) != requestPath || filepath.Clean(configPath) != configPath {
		return fmt.Errorf("offline Resume prepare paths are invalid")
	}
	if _, err := os.Lstat(configPath); !os.IsNotExist(err) {
		return fmt.Errorf("offline Resume configuration already exists or is inaccessible")
	}
	request, err := loadOfflineResumeRequest(requestPath)
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	base, err := prepareCurrentOfflineExperiment(ctx, request.baseExperiment(), executable)
	if err != nil {
		return err
	}
	config := resumeConfigFromBase(request, base.promotion)
	registration, err := NewOfflineResumePreregistration(config.Plan, config.fixedAuthority())
	if err != nil {
		return err
	}
	if err := SealOfflineResumePreregistration(config.Paths().Preregistration, registration); err != nil {
		return err
	}
	config.PreregistrationSHA256, err = registration.SHA256()
	if err != nil {
		return err
	}
	if err := config.ValidateStart(); err != nil {
		return err
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return err
	}
	return writeExclusiveAtomic(configPath, append(raw, '\n'))
}

func LoadOfflineResumeConfig(path string) (OfflineResumeConfig, error) {
	var config OfflineResumeConfig
	if err := loadStrictJSONFile(path, &config, "offline Resume configuration"); err != nil {
		return OfflineResumeConfig{}, err
	}
	if err := config.Validate(); err != nil {
		return OfflineResumeConfig{}, err
	}
	return config, nil
}

func loadOfflineResumeRequest(path string) (OfflineResumeRequest, error) {
	file, err := os.Open(path)
	if err != nil {
		return OfflineResumeRequest{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 ||
		info.Size() > maxOfflineExperimentRequestBytes {
		return OfflineResumeRequest{}, fmt.Errorf("offline Resume request is not one bounded regular file")
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxOfflineExperimentRequestBytes+1))
	if err != nil {
		return OfflineResumeRequest{}, err
	}
	var request OfflineResumeRequest
	if err := decodeStrictJSON(raw, &request, "offline Resume request"); err != nil {
		return OfflineResumeRequest{}, err
	}
	if err := request.Validate(); err != nil {
		return OfflineResumeRequest{}, err
	}
	return request, nil
}

func (request OfflineResumeRequest) baseExperiment() OfflineExperimentRequest {
	return OfflineExperimentRequest{
		Schema: OfflineExperimentRequestSchemaV1, Mode: OfflineExperimentRun,
		Variant: VariantFullCognition, Suite: SuiteCombined, Seed: request.Plan.Seed,
		Surface: request.Plan.Surface, Budget: request.Budget, DatabaseURL: request.DatabaseURL,
		OllamaEndpoint: request.OllamaEndpoint, InferenceTimeoutSeconds: request.InferenceTimeoutSeconds,
		Repetition: request.Plan.Repetition, PublicOutputDirectory: request.PublicOutputDirectory,
		PrivateOutputDirectory: request.PrivateOutputDirectory, Brain: request.Brain,
	}
}

func resumeConfigFromBase(
	request OfflineResumeRequest,
	base OfflinePromotionConfig,
) OfflineResumeConfig {
	return OfflineResumeConfig{
		Schema: OfflineResumeConfigSchemaV2, Plan: request.Plan, Budget: base.Scenario.Budget(),
		DatabaseURL: request.DatabaseURL, OllamaEndpoint: request.OllamaEndpoint,
		InferenceTimeoutSeconds: request.InferenceTimeoutSeconds,
		PublicOutputDirectory:   request.PublicOutputDirectory,
		PrivateOutputDirectory:  request.PrivateOutputDirectory,
		RatGeneration:           base.RatGeneration, RuntimeFingerprint: base.RuntimeFingerprint,
		PreparedBrainEvidence:   base.PreparedBrainEvidence,
		OmnidexCommit: base.OmnidexCommit, LedgerSchemaVersion: base.LedgerSchemaVersion,
		WorkingSetPolicyVersion: base.WorkingSetPolicyVersion,
		ProjectionPolicyVersion: base.ProjectionPolicyVersion,
	}
}
