package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/jackc/pgx/v5"
)

func insertCognitionObligationSupportingRefsTx(
	ctx context.Context,
	tx pgx.Tx,
	episodeID cognition.EpisodeID,
	obligationID cognition.ObligationID,
	refs []cognition.EvidenceRef,
) error {
	for _, ref := range refs {
		raw, digest, err := cognitionJSON(ref)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO cognition_obligation_supporting_refs (
				episode_id,node_id,observation_id,revision_number,revision_sha256,
				content_sha256,ref_json,ref_json_sha256
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		`, episodeID, obligationID, ref.ObservationID, int64(ref.Revision.Number),
			ref.Revision.SHA256, ref.SHA256, string(raw), digest)
		if err != nil {
			return fmt.Errorf("insert cognition obligation %q support: %w", obligationID, err)
		}
	}
	return nil
}

func insertCognitionObligationDependenciesTx(
	ctx context.Context,
	tx pgx.Tx,
	episodeID cognition.EpisodeID,
	obligationID cognition.ObligationID,
	dependencies []cognition.ObligationID,
) error {
	for _, dependency := range dependencies {
		if _, err := tx.Exec(ctx, `
			INSERT INTO cognition_obligation_dependencies (episode_id,node_id,dependency_node_id)
			VALUES ($1,$2,$3)
		`, episodeID, obligationID, dependency); err != nil {
			return fmt.Errorf("insert cognition obligation %q dependency: %w", obligationID, err)
		}
	}
	return nil
}
