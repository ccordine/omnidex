package cognitiongauntlet

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func PrepareOfflineMatrixConfig(
	ctx context.Context,
	requestPath string,
	configPath string,
) error {
	if ctx == nil || requestPath == "" || configPath == "" || requestPath == configPath ||
		filepath.Clean(requestPath) != requestPath || filepath.Clean(configPath) != configPath {
		return fmt.Errorf("offline matrix prepare paths are invalid")
	}
	if _, err := os.Lstat(configPath); !os.IsNotExist(err) {
		return fmt.Errorf("offline matrix configuration already exists or is inaccessible")
	}
	request, err := loadOfflineMatrixRequest(requestPath)
	if err != nil {
		return err
	}
	paths := OfflineMatrixPaths{
		Preregistration: filepath.Join(request.PrivateOutputDirectory, "matrix-preregistration.json"),
		Receipt:         filepath.Join(request.PrivateOutputDirectory, "matrix-receipt.json"),
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve offline matrix prepare executable: %w", err)
	}
	base, err := prepareCurrentOfflineExperiment(ctx, request.baseExperiment(), executable)
	if err != nil {
		return err
	}
	fixed := OfflineMatrixFixedAuthority{
		Budget: base.promotion.Scenario.Budget(), RatGeneration: base.promotion.RatGeneration,
		RuntimeFingerprint:      base.promotion.RuntimeFingerprint,
		InferenceTimeoutSeconds: request.InferenceTimeoutSeconds,
		OmnidexCommit:           base.promotion.OmnidexCommit,
		LedgerSchemaVersion:     base.promotion.LedgerSchemaVersion,
		WorkingSetPolicyVersion: base.promotion.WorkingSetPolicyVersion,
		ProjectionPolicyVersion: base.promotion.ProjectionPolicyVersion,
	}
	registration, err := NewOfflineMatrixPreregistration(request.Plan, fixed)
	if err != nil {
		return err
	}
	if err := SealOfflineMatrixPreregistration(paths.Preregistration, registration); err != nil {
		return err
	}
	registrationSHA256, err := registration.SHA256()
	if err != nil {
		return err
	}
	config := OfflineMatrixConfig{
		Schema: OfflineMatrixConfigSchemaV2, Plan: request.Plan, Budget: fixed.Budget,
		DatabaseURL: request.DatabaseURL, OllamaEndpoint: request.OllamaEndpoint,
		InferenceTimeoutSeconds: request.InferenceTimeoutSeconds,
		PublicOutputDirectory:   request.PublicOutputDirectory,
		PrivateOutputDirectory:  request.PrivateOutputDirectory,
		RatGeneration:           base.promotion.RatGeneration,
		RuntimeFingerprint:      base.promotion.RuntimeFingerprint,
		PreregistrationSHA256:   registrationSHA256,
		OmnidexCommit:           base.promotion.OmnidexCommit,
		LedgerSchemaVersion:     base.promotion.LedgerSchemaVersion,
		WorkingSetPolicyVersion: base.promotion.WorkingSetPolicyVersion,
		ProjectionPolicyVersion: base.promotion.ProjectionPolicyVersion,
	}
	if err := config.ValidateStart(); err != nil {
		return err
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("encode offline matrix configuration: %w", err)
	}
	return writeExclusiveAtomic(configPath, append(raw, '\n'))
}

func LoadOfflineMatrixConfig(path string) (OfflineMatrixConfig, error) {
	var config OfflineMatrixConfig
	if err := loadStrictJSONFile(path, &config, "offline cognition matrix configuration"); err != nil {
		return OfflineMatrixConfig{}, err
	}
	if err := config.Validate(); err != nil {
		return OfflineMatrixConfig{}, err
	}
	return config, nil
}

func loadOfflineMatrixRequest(path string) (OfflineMatrixRequest, error) {
	file, err := os.Open(path)
	if err != nil {
		return OfflineMatrixRequest{}, fmt.Errorf("open offline matrix request: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 ||
		info.Size() > maxOfflineExperimentRequestBytes {
		return OfflineMatrixRequest{}, fmt.Errorf("offline matrix request is not one bounded regular file")
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxOfflineExperimentRequestBytes+1))
	if err != nil {
		return OfflineMatrixRequest{}, err
	}
	var request OfflineMatrixRequest
	if err := decodeStrictJSON(raw, &request, "offline cognition matrix request"); err != nil {
		return OfflineMatrixRequest{}, err
	}
	if err := request.Validate(); err != nil {
		return OfflineMatrixRequest{}, err
	}
	return request, nil
}

func (request OfflineMatrixRequest) baseExperiment() OfflineExperimentRequest {
	return OfflineExperimentRequest{
		Schema: OfflineExperimentRequestSchemaV1, Mode: OfflineExperimentRun,
		Variant: VariantRawObservation, Suite: request.Plan.Suites[0], Seed: request.Plan.Seeds[0],
		Surface: request.Plan.Surface, Budget: request.Budget, DatabaseURL: request.DatabaseURL,
		OllamaEndpoint:          request.OllamaEndpoint,
		InferenceTimeoutSeconds: request.InferenceTimeoutSeconds, Repetition: 1,
		PublicOutputDirectory:  request.PublicOutputDirectory,
		PrivateOutputDirectory: request.PrivateOutputDirectory, Brain: request.Brain,
	}
}
