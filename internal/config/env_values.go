package config

import (
	"os"
	"strconv"
	"time"
)

func getenv(key, fallback string) string {
	value, configured := os.LookupEnv(key)
	if !configured {
		return fallback
	}
	return value
}

func getenvInt(key string, fallback int) int {
	value, configured := os.LookupEnv(key)
	if !configured {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}

func getenvDuration(key string, fallback time.Duration) time.Duration {
	value, configured := os.LookupEnv(key)
	if !configured {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0
	}
	return parsed
}
