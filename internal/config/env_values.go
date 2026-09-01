package config

import "os"

func getenv(key, fallback string) string {
	value, configured := os.LookupEnv(key)
	if !configured {
		return fallback
	}
	return value
}
