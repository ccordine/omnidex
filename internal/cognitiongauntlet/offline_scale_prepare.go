package cognitiongauntlet

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func PrepareOfflineScaleConfig(
	ctx context.Context,
	requestPath string,
	configPath string,
) error {
	if ctx == nil || requestPath == "" || configPath == "" || requestPath == configPath ||
		filepath.Clean(requestPath) != requestPath || filepath.Clean(configPath) != configPath {
		return fmt.Errorf("offline Scale prepare paths are invalid")
	}
	if _, err := os.Lstat(configPath); !os.IsNotExist(err) {
		return fmt.Errorf("offline Scale configuration already exists or is inaccessible")
	}
	request, err := loadOfflineScaleRequest(requestPath)
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve offline Scale executable: %w", err)
	}
	base, err := prepareCurrentOfflineExperiment(ctx, request.baseExperiment(), executable)
	if err != nil {
		return err
	}
	fixed := OfflineMatrixFixedAuthority{
		Budget:                  base.promotion.Scenario.Budget(),
		RatGeneration:           base.promotion.RatGeneration,
		RuntimeFingerprint:      base.promotion.RuntimeFingerprint,
		InferenceTimeoutSeconds: request.InferenceTimeoutSeconds,
		OmnidexCommit:           base.promotion.OmnidexCommit,
		LedgerSchemaVersion:     base.promotion.LedgerSchemaVersion,
		WorkingSetPolicyVersion: base.promotion.WorkingSetPolicyVersion,
		ProjectionPolicyVersion: base.promotion.ProjectionPolicyVersion,
	}
	registration, err := NewOfflineScalePreregistration(request.Plan, fixed)
	if err != nil {
		return err
	}
	paths := OfflineScalePaths{
		Preregistration: filepath.Join(request.PrivateOutputDirectory, "scale-preregistration.json"),
		Receipt:         filepath.Join(request.PrivateOutputDirectory, "scale-receipt.json"),
	}
	if err := SealOfflineScalePreregistration(paths.Preregistration, registration); err != nil {
		return err
	}
	registrationSHA, err := registration.SHA256()
	if err != nil {
		return err
	}
	config := OfflineScaleConfig{
		Schema: OfflineScaleConfigSchemaV1, Plan: request.Plan, Budget: fixed.Budget,
		DatabaseURL: request.DatabaseURL, OllamaEndpoint: request.OllamaEndpoint,
		InferenceTimeoutSeconds: request.InferenceTimeoutSeconds,
		PublicOutputDirectory:   request.PublicOutputDirectory,
		PrivateOutputDirectory:  request.PrivateOutputDirectory,
		RatGeneration:           fixed.RatGeneration, RuntimeFingerprint: fixed.RuntimeFingerprint,
		PreregistrationSHA256: registrationSHA, OmnidexCommit: fixed.OmnidexCommit,
		LedgerSchemaVersion:     fixed.LedgerSchemaVersion,
		WorkingSetPolicyVersion: fixed.WorkingSetPolicyVersion,
		ProjectionPolicyVersion: fixed.ProjectionPolicyVersion,
	}
	if err := config.ValidateStart(); err != nil {
		return err
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("encode offline Scale configuration: %w", err)
	}
	return writeExclusiveAtomic(configPath, append(raw, '\n'))
}

func LoadOfflineScaleConfig(path string) (OfflineScaleConfig, error) {
	var config OfflineScaleConfig
	if err := loadStrictJSONFile(path, &config, "offline Scale configuration"); err != nil {
		return OfflineScaleConfig{}, err
	}
	if err := config.Validate(); err != nil {
		return OfflineScaleConfig{}, err
	}
	return config, nil
}

func loadOfflineScaleRequest(path string) (OfflineScaleRequest, error) {
	file, err := os.Open(path)
	if err != nil {
		return OfflineScaleRequest{}, fmt.Errorf("open offline Scale request: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 ||
		info.Size() > maxOfflineExperimentRequestBytes {
		return OfflineScaleRequest{}, fmt.Errorf("offline Scale request is not one bounded regular file")
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxOfflineExperimentRequestBytes+1))
	if err != nil {
		return OfflineScaleRequest{}, err
	}
	var request OfflineScaleRequest
	if err := decodeStrictJSON(raw, &request, "offline Scale request"); err != nil {
		return OfflineScaleRequest{}, err
	}
	if err := request.Validate(); err != nil {
		return OfflineScaleRequest{}, err
	}
	return request, nil
}

func (request OfflineScaleRequest) baseExperiment() OfflineExperimentRequest {
	return OfflineExperimentRequest{
		Schema: OfflineExperimentRequestSchemaV1, Mode: OfflineExperimentRun,
		Variant: VariantFullCognition, Suite: SuiteCombined, Seed: request.Plan.Seed,
		Surface: SurfaceSymbolic, Budget: request.Budget,
		DatabaseURL: request.DatabaseURL, OllamaEndpoint: request.OllamaEndpoint,
		InferenceTimeoutSeconds: request.InferenceTimeoutSeconds, Repetition: 1,
		PublicOutputDirectory:  request.PublicOutputDirectory,
		PrivateOutputDirectory: request.PrivateOutputDirectory, Brain: request.Brain,
	}
}
