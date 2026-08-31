package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/omni"
)

func main() {
	if err := applyInvocationCWDFromEnv(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	app := omni.NewApp(os.Stdin, os.Stdout, os.Stderr)
	if err := app.Run(os.Args[1:]); err != nil {
		if code, ok := omni.IsExitCodeError(err); ok {
			os.Exit(code)
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

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
