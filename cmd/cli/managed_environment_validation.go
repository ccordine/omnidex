package main

import (
	"fmt"

	"github.com/gryph/omnidex/internal/envfile"
)

func validateManagedCLIEnvironmentFile(path string) error {
	values, err := envfile.Read(path)
	if err != nil {
		return fmt.Errorf("read managed environment: %w", err)
	}
	coreURL, found := values["CORE_URL"]
	if !found {
		return fmt.Errorf("managed environment does not define CORE_URL")
	}
	if _, err := normalizedCoreURL(coreURL); err != nil {
		return fmt.Errorf("managed CORE_URL: %w", err)
	}
	return nil
}
