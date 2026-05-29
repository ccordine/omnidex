package worker

import (
	"encoding/json"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
)

func modelRoutingFromJobMetadata(metadata json.RawMessage, base ModelRouting) ModelRouting {
	if len(metadata) == 0 {
		return base
	}
	var payload map[string]any
	if err := json.Unmarshal(metadata, &payload); err != nil {
		return base
	}
	cfg := modelconfig.Config{}
	if raw, ok := payload["model_config"]; ok {
		bytes, err := json.Marshal(raw)
		if err == nil {
			cfg = modelconfig.FromJSON(bytes)
		}
	}
	if len(cfg) == 0 {
		return base
	}
	baseRouting := modelconfig.Routing{
		Default:    base.Default,
		Fast:       base.Fast,
		Reasoning:  base.Reasoning,
		Tagging:    base.Tagging,
		Plan:       base.Plan,
		Analyze:    base.Analyze,
		Response:   base.Response,
		Search:     base.Search,
		Memory:     base.Memory,
		Specialist: base.Specialist,
	}
	applied := modelconfig.Apply(baseRouting, cfg)
	return ModelRouting{
		Default:    applied.Default,
		Fast:       applied.Fast,
		Reasoning:  applied.Reasoning,
		Tagging:    applied.Tagging,
		Plan:       applied.Plan,
		Analyze:    applied.Analyze,
		Response:   applied.Response,
		Search:     applied.Search,
		Memory:     applied.Memory,
		Specialist: applied.Specialist,
	}
}

func modelConfigSource(metadata json.RawMessage) string {
	source := strings.TrimSpace(metadataString(metadata, "model_config_source"))
	if source == "" {
		return "env"
	}
	return source
}

func withJobModelRouting(s *Service, job model.Job) func() {
	if s == nil {
		return func() {}
	}
	prev := s.models
	s.models = modelRoutingFromJobMetadata(job.Metadata, prev)
	return func() {
		s.models = prev
	}
}
