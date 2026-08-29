package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func getenvInt(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		parsed, err := strconv.Atoi(value)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func requiredPositiveEnvInt(key string) (int, error) {
	raw, exists := os.LookupEnv(key)
	if !exists || strings.TrimSpace(raw) == "" {
		return 0, fmt.Errorf("%s is required", key)
	}
	if raw != strings.TrimSpace(raw) {
		return 0, fmt.Errorf("%s must be one exact positive integer", key)
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be one exact positive integer, received %q", key, raw)
	}
	return value, nil
}

func getenvBool(key string, fallback bool) bool {
	if value := os.Getenv(key); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func getenvDuration(key string, fallback time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		parsed, err := time.ParseDuration(value)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func getenvCSV(key string, fallback []string) []string {
	value := os.Getenv(key)
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	out := make([]string, 0)
	for _, part := range strings.Split(value, ",") {
		if clean := strings.TrimSpace(part); clean != "" {
			out = append(out, clean)
		}
	}
	if len(out) == 0 {
		return fallback
	}
	return out
}
