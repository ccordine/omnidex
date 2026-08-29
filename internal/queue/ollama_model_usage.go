package queue

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/modelref"
)

type OllamaRoleplayModelUsage struct {
	NarrativeCharacters int64
	VoiceCharacters     int64
}

func (usage OllamaRoleplayModelUsage) InUse() bool {
	return usage.NarrativeCharacters > 0 || usage.VoiceCharacters > 0
}

func (r *Repository) RoleplayOllamaModelUsage(
	ctx context.Context,
	installedModel string,
) (OllamaRoleplayModelUsage, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return OllamaRoleplayModelUsage{}, fmt.Errorf("roleplay model usage requires PostgreSQL and context")
	}
	if err := modelref.ValidateOllamaName(installedModel); err != nil {
		return OllamaRoleplayModelUsage{}, err
	}
	base := installedModel
	if separator := strings.LastIndex(installedModel, ":"); separator > 0 {
		base = installedModel[:separator]
	}
	var usage OllamaRoleplayModelUsage
	err := r.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE narrative_model IN ($1,$2)),
			COUNT(*) FILTER (WHERE voice_rewrite_enabled AND voice_rewrite_model IN ($1,$2))
		FROM roleplay_character_generation_configs
	`, installedModel, base).Scan(&usage.NarrativeCharacters, &usage.VoiceCharacters)
	if err != nil {
		return OllamaRoleplayModelUsage{}, err
	}
	return usage, nil
}
