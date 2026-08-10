package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) StartCognitionEnvironment(
	ctx context.Context,
	episode cognition.EpisodeRef,
	scenario cognition.ScenarioRef,
	start cognition.Transition,
) (cognition.Transition, error) {
	if r == nil || r.pool == nil || ctx == nil {
		return cognition.Transition{}, fmt.Errorf("cognition environment journal requires PostgreSQL and context")
	}
	if err := validateEnvironmentStart(episode, scenario, start); err != nil {
		return cognition.Transition{}, err
	}
	startRaw, startSHA, err := cognitionJSON(start)
	if err != nil {
		return cognition.Transition{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return cognition.Transition{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		INSERT INTO cognition_environment_journals (
			episode_id,scenario_id,scenario_sha256,start_json,start_sha256,
			current_revision,current_revision_sha256,terminal
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (episode_id) DO NOTHING
	`, episode.ID, scenario.ID, scenario.SHA256, string(startRaw), startSHA,
		int64(start.Current.Number), start.Current.SHA256, start.Terminal); err != nil {
		return cognition.Transition{}, fmt.Errorf("start cognition environment journal: %w", err)
	}
	state, found, err := loadCognitionEnvironmentJournalTx(ctx, tx, episode, true)
	if err != nil {
		return cognition.Transition{}, err
	}
	if !found || state.Scenario != scenario {
		return cognition.Transition{}, cognition.ErrEnvironmentJournalConflict
	}
	existingRaw, existingSHA, err := cognitionJSON(state.Start)
	if err != nil || existingSHA != startSHA || string(existingRaw) != string(startRaw) {
		return cognition.Transition{}, cognition.ErrEnvironmentJournalConflict
	}
	if err := tx.Commit(ctx); err != nil {
		return cognition.Transition{}, err
	}
	return state.Start.Clone(), nil
}
