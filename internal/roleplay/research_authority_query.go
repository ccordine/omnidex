package roleplay

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func loadResearchPreparationTx(
	ctx context.Context,
	tx pgx.Tx,
	preparationID string,
) (ResearchTurnAuthority, bool, error) {
	return scanResearchAuthority(tx.QueryRow(ctx, researchAuthoritySelect(true)+`
		WHERE research.preparation_id=$1
	`, preparationID))
}

type researchRowQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func loadResearchJob(
	ctx context.Context,
	querier researchRowQuerier,
	jobID int64,
	requireActive bool,
) (ResearchTurnAuthority, bool, error) {
	return scanResearchAuthority(querier.QueryRow(ctx, researchAuthoritySelect(requireActive)+`
		JOIN roleplay_research_preparation_jobs AS binding
		  ON binding.preparation_id=research.preparation_id
		WHERE binding.job_id=$1
	`, jobID))
}

const researchAuthorityBaseSelect = `
	SELECT 'omnidex.roleplay-research-turn.v1',research.preparation_id,research.channel_id,
	       research.user_message_id,research.world_id,research.scene_id,
	       research.scene_revision,research.character_id,research.capability,
	       research.capability_grant_id,research.question,research.question_sha256,
	       research.narrative_fingerprint,research.authority_namespace,
	       capability_grant.created_at,research.created_at
	FROM roleplay_research_turns AS research
	JOIN roleplay_character_capability_grants AS capability_grant
	  ON capability_grant.grant_id=research.capability_grant_id
	 AND capability_grant.world_id=research.world_id
	 AND capability_grant.character_id=research.character_id
	 AND capability_grant.capability=research.capability `

func researchAuthoritySelect(requireActive bool) string {
	if !requireActive {
		return researchAuthorityBaseSelect
	}
	return researchAuthorityBaseSelect + `
	JOIN roleplay_character_capabilities AS active
	  ON active.grant_id=research.capability_grant_id
	 AND active.world_id=research.world_id
	 AND active.character_id=research.character_id
	 AND active.capability=research.capability `
}

func scanResearchAuthority(row pgx.Row) (ResearchTurnAuthority, bool, error) {
	var authority ResearchTurnAuthority
	var namespace string
	err := row.Scan(
		&authority.Schema, &authority.PreparationID, &authority.ChannelID, &authority.UserMessageID,
		&authority.WorldID, &authority.SceneID, &authority.SceneRevision, &authority.CharacterID,
		&authority.Capability, &authority.CapabilityGrantID, &authority.Question,
		&authority.QuestionSHA256, &authority.NarrativeFingerprint, &namespace,
		&authority.CapabilityIssuedAt, &authority.CreatedAt,
	)
	if err == pgx.ErrNoRows {
		return ResearchTurnAuthority{}, false, nil
	}
	if err != nil {
		return ResearchTurnAuthority{}, false, err
	}
	authority.Authority = AuthorityNamespace(namespace)
	if err := authority.Validate(); err != nil {
		return ResearchTurnAuthority{}, false, fmt.Errorf("persisted roleplay research authority: %w", err)
	}
	return authority, true, nil
}
