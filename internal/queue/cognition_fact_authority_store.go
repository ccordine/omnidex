package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/jackc/pgx/v5"
)

func cognitionFactAuthorityIdentity(ref cognitionstate.FactAcceptanceAuthorityRef) any {
	return struct {
		Schema   string                                   `json:"schema"`
		Planner  cognitionstate.FactPlannerRef            `json:"planner"`
		Policies []cognitionstate.FactAcceptancePolicyRef `json:"policies"`
	}{ref.Schema, ref.Planner, append([]cognitionstate.FactAcceptancePolicyRef{}, ref.Policies...)}
}

func insertCognitionFactAuthorityTx(
	ctx context.Context,
	tx pgx.Tx,
	episodeID cognition.EpisodeID,
	ref cognitionstate.FactAcceptanceAuthorityRef,
) error {
	if err := ref.Validate(); err != nil {
		return fmt.Errorf("persist cognition fact authority: %w", err)
	}
	for position, policy := range ref.Policies {
		if _, err := tx.Exec(ctx, `
			INSERT INTO cognition_episode_fact_policies (
				episode_id,position,policy_id,policy_version,policy_sha256
			) VALUES ($1,$2,$3,$4,$5)
		`, episodeID, position, policy.ID, policy.Version, policy.SHA256); err != nil {
			return fmt.Errorf("persist cognition fact policy %q: %w", policy.ID, err)
		}
	}
	return nil
}
