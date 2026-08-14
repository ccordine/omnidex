package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRequestTimeoutDefaultsToTenMinutesEverywhere(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("WRAPPER_ONLY", "true")
	t.Setenv("REQUEST_TIMEOUT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RequestTimeout != 10*time.Minute {
		t.Fatalf("RequestTimeout=%s want 10m", cfg.RequestTimeout)
	}

	root := filepath.Clean(filepath.Join("..", ".."))
	for path, exact := range map[string]string{
		"default.env":        "REQUEST_TIMEOUT=10m",
		".env.example":       "REQUEST_TIMEOUT=10m",
		"docker-compose.yml": "REQUEST_TIMEOUT: ${REQUEST_TIMEOUT:-10m}",
	} {
		raw, readErr := os.ReadFile(filepath.Join(root, path))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if count := strings.Count(string(raw), exact); count != 1 {
			t.Errorf("%s contains %d copies of %q, want exactly one", path, count, exact)
		}
	}
}

func TestRequestTimeoutExplicitOverrideRemainsAuthoritative(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("WRAPPER_ONLY", "true")
	t.Setenv("REQUEST_TIMEOUT", "275ms")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RequestTimeout != 275*time.Millisecond {
		t.Fatalf("RequestTimeout=%s want 275ms", cfg.RequestTimeout)
	}
}
