package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/config"
	"github.com/gryph/omnidex/internal/envfile"
)

func validateCoreEnvironmentFile(path string) error {
	values, err := envfile.Read(path)
	if err != nil {
		return fmt.Errorf("read environment file: %w", err)
	}
	previous := append([]string(nil), os.Environ()...)
	os.Clearenv()
	defer restoreProcessEnvironment(previous)

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := os.Setenv(key, values[key]); err != nil {
			return fmt.Errorf("set environment key %s: %w", key, err)
		}
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	return config.Validate(cfg)
}

func restoreProcessEnvironment(previous []string) {
	os.Clearenv()
	for _, entry := range previous {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			_ = os.Setenv(key, value)
		}
	}
}
