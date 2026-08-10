package main

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/config"
)

func TestCognitionBrainFromConfigBindsExactAnalyzeRouteAndSampling(t *testing.T) {
	cfg := config.Config{
		LLMProvider: "ollama", AnalyzeModel: "qwen3.5:9b-q4_K_M",
		InferenceContextTokens: 32768, CognitionContextCeilingBytes: 24576,
		CognitionMaxOutputTokens:   4096,
		CognitionModelDigest:       strings.Repeat("a", 64),
		CognitionModelQuantization: "Q4_K_M", CognitionBackendVersion: "0.24.0",
		CognitionHardware: "integration-hardware",
	}
	brain, err := cognitionBrainFromConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if brain.Model != cfg.AnalyzeModel || brain.Backend != cfg.LLMProvider ||
		brain.Sampling.MaxOutputTokens != cfg.CognitionMaxOutputTokens ||
		brain.SamplingSHA256 == "" {
		t.Fatalf("brain=%+v", brain)
	}
	cfg.AnalyzeModel = ""
	if _, err := cognitionBrainFromConfig(cfg); err == nil {
		t.Fatal("empty analyze route produced a cognition brain")
	}
}
