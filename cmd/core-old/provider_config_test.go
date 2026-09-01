package main

import (
	"os"
	"strings"
	"testing"
)

func TestCoreBuildsLazyProviderResolversAfterStoredSecretOverlay(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read core startup source: %v", err)
	}
	text := string(source)
	overlay := strings.Index(text, "secrets.OverlayConfig(&cfg, secretResolver)")
	validation := strings.Index(text, "config.Validate(cfg)")
	transports := strings.Index(text, "llmprovider.NewLazyFromConfig(cfg)")
	workerStations := strings.Index(text, "llmTransports.Stations")
	workerEmbeddings := strings.Index(text, "llmTransports.Embeddings")
	if overlay < 0 || validation < 0 || transports < 0 || workerStations < 0 || workerEmbeddings < 0 {
		t.Fatalf("startup is missing overlay, validation, typed transport construction, or worker wiring: overlay=%d validation=%d transports=%d stations=%d embeddings=%d", overlay, validation, transports, workerStations, workerEmbeddings)
	}
	if !(overlay < validation && validation < transports && transports < workerStations) {
		t.Fatalf("provider startup order must be stored-secret overlay -> runtime validation -> lazy resolver construction -> worker wiring: overlay=%d validation=%d transports=%d stations=%d", overlay, validation, transports, workerStations)
	}
	if strings.Contains(text, "llmprovider.NewFromConfig") {
		t.Fatal("eager provider construction remains in core startup")
	}
}
