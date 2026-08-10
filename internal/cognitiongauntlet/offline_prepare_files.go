package cognitiongauntlet

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const maxOfflineExperimentRequestBytes = 256 * 1024

func PrepareOfflineExperimentConfig(
	ctx context.Context,
	requestPath string,
	configPath string,
) error {
	if requestPath == "" || configPath == "" || requestPath == configPath ||
		filepath.Clean(requestPath) != requestPath || filepath.Clean(configPath) != configPath {
		return fmt.Errorf("offline prepare requires distinct exact request and configuration paths")
	}
	request, err := loadOfflineExperimentRequest(requestPath)
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve offline prepare executable: %w", err)
	}
	prepared, err := prepareCurrentOfflineExperiment(ctx, request, executable)
	if err != nil {
		return err
	}
	return sealPreparedOfflineExperiment(configPath, prepared)
}

func sealPreparedOfflineExperiment(configPath string, prepared preparedOfflineExperiment) error {
	if prepared.mode == OfflineExperimentRun {
		if err := prepared.promotion.Validate(); err != nil {
			return err
		}
	} else if prepared.mode == OfflineExperimentTakeover {
		if err := prepared.takeover.Validate(); err != nil {
			return err
		}
	} else {
		return fmt.Errorf("offline prepared configuration mode is not registered")
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		return fmt.Errorf("offline prepared configuration already exists or is inaccessible")
	}
	parent, err := os.Stat(filepath.Dir(configPath))
	if err != nil || !parent.IsDir() {
		return fmt.Errorf("offline prepared configuration directory is unavailable")
	}
	var value any = prepared.promotion
	if prepared.mode == OfflineExperimentTakeover {
		value = prepared.takeover
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode offline prepared configuration: %w", err)
	}
	return writeExclusiveAtomic(configPath, append(raw, '\n'))
}

func loadOfflineExperimentRequest(path string) (OfflineExperimentRequest, error) {
	file, err := os.Open(path)
	if err != nil {
		return OfflineExperimentRequest{}, fmt.Errorf("open offline experiment request: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 ||
		info.Size() > maxOfflineExperimentRequestBytes {
		return OfflineExperimentRequest{}, fmt.Errorf("offline experiment request is not one bounded regular file")
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxOfflineExperimentRequestBytes+1))
	if err != nil {
		return OfflineExperimentRequest{}, fmt.Errorf("read bounded offline experiment request: %w", err)
	}
	if len(raw) == 0 || len(raw) > maxOfflineExperimentRequestBytes {
		return OfflineExperimentRequest{}, fmt.Errorf("offline experiment request byte count is invalid")
	}
	request := OfflineExperimentRequest{}
	if err := decodeStrictJSON(raw, &request, "offline cognition experiment request"); err != nil {
		return OfflineExperimentRequest{}, err
	}
	if err := request.Validate(); err != nil {
		return OfflineExperimentRequest{}, err
	}
	return request, nil
}
