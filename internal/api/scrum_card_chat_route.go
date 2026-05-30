package api

import (
	"context"

	"github.com/gryph/omnidex/internal/agentconfig"
	"github.com/gryph/omnidex/internal/model"
)

func (s *Server) scrumCardResolvedAgent(ctx context.Context, projectID int64, card ScrumCard) agentconfig.Config {
	if isScrumExternalCard(card) {
		return agentconfig.FromJSON(card.AgentConfig)
	}
	project := model.Project{}
	if s.repo != nil && projectID > 0 {
		if loaded, err := s.repo.GetProject(ctx, projectID); err == nil {
			project = loaded
		}
	}
	resolved, _ := s.resolveAgentConfig(ctx, project, card)
	return resolved
}

func (s *Server) scrumCardUsesExternalAgent(ctx context.Context, projectID int64, card ScrumCard) bool {
	return s.scrumCardResolvedAgent(ctx, projectID, card).IsExternal()
}
