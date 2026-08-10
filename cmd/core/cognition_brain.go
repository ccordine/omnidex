package main

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/config"
)

func cognitionBrainFromConfig(cfg config.Config) (cognitionpolicy.BrainRef, error) {
	sampling, err := cognitionpolicy.NewSamplingIdentity(
		cfg.InferenceContextTokens,
		cfg.CognitionContextCeilingBytes,
		cfg.CognitionMaxOutputTokens,
	)
	if err != nil {
		return cognitionpolicy.BrainRef{}, fmt.Errorf("build cognition sampling identity: %w", err)
	}
	brain, err := cognitionpolicy.NewBrainRef(
		cfg.AnalyzeModel,
		cfg.CognitionModelDigest,
		cfg.CognitionModelQuantization,
		cfg.LLMProvider,
		cfg.CognitionBackendVersion,
		cfg.CognitionHardware,
		sampling,
	)
	if err != nil {
		return cognitionpolicy.BrainRef{}, fmt.Errorf("build cognition brain identity: %w", err)
	}
	return brain, nil
}
