package queue

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func insertCognitionAcceptedFactTx(
	ctx context.Context, tx pgx.Tx, value CognitionAcceptedFactTrace,
) error {
	if err := value.validate(); err != nil {
		return err
	}
	raw, rawSHA, err := cognitionJSON(value)
	if err != nil {
		return err
	}
	identityRaw, identitySHA, err := cognitionJSON(value.identity())
	if err != nil || identitySHA != value.SHA256 {
		return fmt.Errorf("%w: accepted cognition fact identity changed", ErrCognitionConflict)
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO cognition_accepted_facts (
			fact_id,fact_sha256,episode_id,ledger_id,transition_id,transition_sha256,
			scope_obligation_id,authority_sha256,planner_id,planner_version,planner_sha256,
			policy_id,policy_version,policy_sha256,entry_id,command_id,command_sha256,
			identity_json,identity_json_sha256,descriptor_json,descriptor_json_sha256
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)
	`, value.ID, value.SHA256, value.EpisodeID, value.LedgerID,
		value.TransitionID, value.TransitionSHA256, value.ScopeObligationID, value.AuthoritySHA256,
		value.Planner.ID, value.Planner.Version, value.Planner.SHA256,
		value.Policy.ID, value.Policy.Version, value.Policy.SHA256,
		value.Mapping.EntryID, value.Mapping.CommandID, value.Mapping.CommandSHA256,
		string(identityRaw), identitySHA, string(raw), rawSHA)
	if err != nil {
		return fmt.Errorf("insert cognition accepted fact: %w", err)
	}
	for index, ref := range value.EvidenceRefs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO cognition_accepted_fact_evidence (
				fact_id,position,observation_id,revision,revision_sha256,content_sha256
			) VALUES ($1,$2,$3,$4,$5,$6)
		`, value.ID, index, ref.ObservationID, int64(ref.Revision.Number),
			ref.Revision.SHA256, ref.SHA256); err != nil {
			return fmt.Errorf("insert cognition accepted fact evidence: %w", err)
		}
	}
	return nil
}
