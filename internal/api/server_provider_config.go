package api

import "github.com/gryph/omnidex/internal/config"

func (s *Server) providerConfiguration() config.Config {
	if s == nil {
		return config.Config{}
	}
	cfg := s.providerConfig
	cfg.CompatibleProviders = config.CloneCompatibleProviders(cfg.CompatibleProviders)
	cfg.ProviderModels = config.CloneProviderModels(cfg.ProviderModels)
	return cfg
}
