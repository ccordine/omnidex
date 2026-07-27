package main

import (
	"os"
	"strings"
	"testing"
)

func TestCoreValidatesProviderCredentialsAfterStoredSecretOverlay(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read core startup source: %v", err)
	}
	text := string(source)
	overlay := strings.Index(text, "secrets.OverlayConfig(&cfg, secretResolver)")
	validation := strings.Index(text, "config.Validate(cfg)")
	client := strings.Index(text, "llmprovider.NewFromConfig(cfg)")
	if overlay < 0 || validation < 0 || client < 0 {
		t.Fatalf("startup is missing overlay, validation, or provider construction: overlay=%d validation=%d client=%d", overlay, validation, client)
	}
	if !(overlay < validation && validation < client) {
		t.Fatalf("provider startup order must be stored-secret overlay -> validation -> construction: overlay=%d validation=%d client=%d", overlay, validation, client)
	}
}
