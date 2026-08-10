package cognitiongauntlet

import (
	"encoding/json"
	"fmt"
	"os"
)

func LoadOfflinePromotionConfig(path string) (OfflinePromotionConfig, error) {
	var config OfflinePromotionConfig
	if err := loadStrictJSONFile(path, &config, "offline cognition promotion configuration"); err != nil {
		return OfflinePromotionConfig{}, err
	}
	if err := config.Validate(); err != nil {
		return OfflinePromotionConfig{}, err
	}
	return config, nil
}

func LoadOfflineTakeoverConfig(path string) (OfflineTakeoverConfig, error) {
	var config OfflineTakeoverConfig
	if err := loadStrictJSONFile(path, &config, "offline cognition takeover configuration"); err != nil {
		return OfflineTakeoverConfig{}, err
	}
	if err := config.Validate(); err != nil {
		return OfflineTakeoverConfig{}, err
	}
	return config, nil
}

func loadStrictJSONFile(path string, target any, label string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", label, err)
	}
	return decodeStrictJSON(raw, target, label)
}

func writePrivateProcessFile(path string, value any, label string) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode %s: %w", label, err)
	}
	return writeExclusiveAtomic(path, append(raw, '\n'))
}
