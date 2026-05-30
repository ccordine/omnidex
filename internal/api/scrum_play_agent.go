package api

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/agentconfig"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/omni"
)

func (s *Server) validateScrumPlayAgent(ctx context.Context, project model.Project, card ScrumCard, instance agentconfig.Config) error {
	resolved, _ := s.resolveAgentConfig(ctx, project, card, instance)
	if !resolved.IsExternal() {
		return nil
	}
	explicit := true
	switch resolved.System() {
	case agentconfig.SystemCursor:
		if omni.NewCursorSDKArchitectAgent(explicit) == nil {
			reason := omni.CursorSDKUnavailableReason(explicit)
			if reason == "" {
				reason = "Cursor SDK is not available"
			}
			return fmt.Errorf("%s\nAdd the Cursor API key under Admin → API secrets (DB) or set CURSOR_API_KEY in env.", reason)
		}
	case agentconfig.SystemCodex:
		if omni.NewCodexSDKArchitectAgent(explicit) == nil {
			reason := omni.CodexSDKUnavailableReason(explicit)
			if reason == "" {
				reason = "Codex SDK is not available"
			}
			return fmt.Errorf("%s\nAdd the Codex API key under Admin → API secrets (DB) or set CODEX_API_KEY in env.", reason)
		}
	default:
		return nil
	}
	return nil
}

func scrumAgentConfigErrorNote(output string) string {
	lower := strings.ToLower(output)
	if !strings.Contains(lower, "strict external agent required") {
		return ""
	}
	if strings.Contains(lower, "cursor sdk") || strings.Contains(lower, "cursor api key") {
		return "play: Cursor API key missing — set it in Admin → API secrets (DB) or CURSOR_API_KEY in env"
	}
	if strings.Contains(lower, "codex sdk") || strings.Contains(lower, "codex api key") {
		return "play: Codex API key missing — set it in Admin → API secrets (DB) or CODEX_API_KEY in env"
	}
	return "play: external execution agent not configured in core"
}
