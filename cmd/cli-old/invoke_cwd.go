package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const invokeCWDEnv = "OMNI_INVOKE_CWD"

func applyInvocationCWDFromEnv() error {
	target := strings.TrimSpace(os.Getenv(invokeCWDEnv))
	if target == "" {
		return nil
	}

	if !filepath.IsAbs(target) {
		abs, err := filepath.Abs(target)
		if err != nil {
			return fmt.Errorf("resolve %s: %w", invokeCWDEnv, err)
		}
		target = abs
	}

	info, err := os.Stat(target)
	if err != nil {
		return fmt.Errorf("invalid %s %q: %w", invokeCWDEnv, target, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("invalid %s %q: not a directory", invokeCWDEnv, target)
	}
	if err := os.Chdir(target); err != nil {
		return fmt.Errorf("chdir %s=%q: %w", invokeCWDEnv, target, err)
	}
	return nil
}
