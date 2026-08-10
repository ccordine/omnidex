package cognitiongauntlet

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func PrepareOfflineTransferConfig(
	ctx context.Context,
	requestPath string,
	configPath string,
) error {
	if ctx == nil || requestPath == "" || configPath == "" || requestPath == configPath ||
		filepath.Clean(requestPath) != requestPath || filepath.Clean(configPath) != configPath {
		return fmt.Errorf("offline Transfer prepare paths are invalid")
	}
	if _, err := os.Lstat(configPath); !os.IsNotExist(err) {
		return fmt.Errorf("offline Transfer configuration already exists or is inaccessible")
	}
	request, err := loadOfflineTransferRequest(requestPath)
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve offline Transfer executable: %w", err)
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
	registration, err := NewOfflineTransferPreregistration(request.Plan, fixed)
	if err != nil {
		return err
	}
	paths := OfflineTransferPaths{
		Preregistration: filepath.Join(request.PrivateOutputDirectory, "transfer-preregistration.json"),
		Receipt:         filepath.Join(request.PrivateOutputDirectory, "transfer-receipt.json"),
	}
	if err := SealOfflineTransferPreregistration(paths.Preregistration, registration); err != nil {
		return err
	}
	registrationSHA, err := registration.SHA256()
	if err != nil {
		return err
	}
	config := OfflineTransferConfig{
		Schema: OfflineTransferConfigSchemaV1, Plan: registration.Plan, Budget: fixed.Budget,
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
		return fmt.Errorf("encode offline Transfer configuration: %w", err)
	}
	return writeExclusiveAtomic(configPath, append(raw, '\n'))
}

func LoadOfflineTransferConfig(path string) (OfflineTransferConfig, error) {
	var config OfflineTransferConfig
	if err := loadStrictJSONFile(path, &config, "offline Transfer configuration"); err != nil {
		return OfflineTransferConfig{}, err
	}
	if err := config.Validate(); err != nil {
		return OfflineTransferConfig{}, err
	}
	return config, nil
}

func loadOfflineTransferRequest(path string) (OfflineTransferRequest, error) {
	file, err := os.Open(path)
	if err != nil {
		return OfflineTransferRequest{}, fmt.Errorf("open offline Transfer request: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 ||
		info.Size() > maxOfflineExperimentRequestBytes {
		return OfflineTransferRequest{}, fmt.Errorf("offline Transfer request is not one bounded regular file")
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxOfflineExperimentRequestBytes+1))
	if err != nil {
		return OfflineTransferRequest{}, err
	}
	var request OfflineTransferRequest
	if err := decodeStrictJSON(raw, &request, "offline Transfer request"); err != nil {
		return OfflineTransferRequest{}, err
	}
	if err := request.Validate(); err != nil {
		return OfflineTransferRequest{}, err
	}
	return request, nil
}

func (request OfflineTransferRequest) baseExperiment() OfflineExperimentRequest {
	return OfflineExperimentRequest{
		Schema: OfflineExperimentRequestSchemaV1, Mode: OfflineExperimentRun,
		Variant: VariantFullCognition, Suite: request.Plan.Suite, Seed: request.Plan.Seed,
		Surface: request.Plan.Surfaces[0], Budget: request.Budget,
		DatabaseURL: request.DatabaseURL, OllamaEndpoint: request.OllamaEndpoint,
		InferenceTimeoutSeconds: request.InferenceTimeoutSeconds,
		Repetition:              request.Plan.Repetition,
		PublicOutputDirectory:   request.PublicOutputDirectory,
		PrivateOutputDirectory:  request.PrivateOutputDirectory, Brain: request.Brain,
	}
}
