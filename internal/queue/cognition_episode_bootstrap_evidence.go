package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/jackc/pgx/v5"
)

func insertCognitionEpisodeBootstrapEvidenceTx(
	ctx context.Context,
	tx pgx.Tx,
	episodeID cognition.EpisodeID,
	bootstrap cognitionpolicy.BrainBootstrap,
) error {
	if err := bootstrap.Validate(); err != nil {
		return err
	}
	if err := insertCognitionProviderIdentityEvidenceBodyTx(
		ctx, tx, bootstrap.BootstrapEvidence,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO cognition_episode_provider_identity_evidence (episode_id,evidence_id)
		VALUES ($1,$2)
	`, episodeID, bootstrap.BootstrapEvidence.Ref.ID); err != nil {
		return fmt.Errorf("associate cognition episode bootstrap provider evidence: %w", err)
	}
	return nil
}
