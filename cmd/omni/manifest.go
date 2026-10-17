package main

import (
	"os"
	"strings"

	"github.com/gryph/omnidex/internal/manifest"
)

func clientCoreURL() (string, error) {
	defaults, configured, err := manifest.LoadHome()
	if err != nil {
		return "", err
	}
	if configured {
		return defaults.CoreURL, nil
	}
	if explicit := strings.TrimSpace(os.Getenv("CORE_URL")); explicit != "" {
		return explicit, nil
	}
	return defaultCoreURL, nil
}
