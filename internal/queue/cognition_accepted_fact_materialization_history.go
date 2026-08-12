package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/jackc/pgx/v5"
)

func requireCognitionAcceptedFactMaterializationReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	episodeID cognition.EpisodeID,
	transition cognition.Transition,
	authority cognitionstate.FactAcceptanceAuthority,
) error {
	_, transitionSHA, err := cognitionJSON(transition)
	if err != nil {
		return err
	}
	var raw []byte
	var payloadSHA string
	if err := tx.QueryRow(ctx, `
		SELECT payload_json,payload_json_sha256
		FROM cognition_accepted_fact_materializations
		WHERE episode_id=$1 AND transition_id=$2
	`, episodeID, cognitionTransitionID(episodeID, transitionSHA)).Scan(&raw, &payloadSHA); err != nil {
		return fmt.Errorf("load accepted-fact materialization replay: %w", err)
	}
	value, err := DecodeCognitionAcceptedFactMaterialization(raw, payloadSHA)
	if err != nil {
		return err
	}
	if err := VerifyCognitionAcceptedFactMaterialization(value, transition, authority); err != nil {
		return fmt.Errorf("verify accepted-fact materialization replay: %w", err)
	}
	return nil
}
