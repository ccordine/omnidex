package api

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/agentconfig"
)

func (s *Server) scrumCardResolvedAgent(ctx context.Context, projectID int64, card ScrumCard) (agentconfig.Config, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("Scrum card agent resolution requires a PostgreSQL repository")
	}
	if ctx == nil {
		return nil, fmt.Errorf("Scrum card agent resolution requires a context")
	}
	if projectID <= 0 {
		return nil, fmt.Errorf("Scrum card agent resolution requires a positive project ID")
	}
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("load project %d agent configuration: %w", projectID, err)
	}
	resolved, _, err := s.resolveAgentConfig(ctx, project, card)
	if err != nil {
		return nil, err
	}
	return resolved, nil
}
