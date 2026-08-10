package host

import (
	"bytes"
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/jackc/pgx/v5"
)

func (environment *Environment) Start(
	ctx context.Context,
	reference cognition.ScenarioRef,
) (cognition.Transition, error) {
	if err := environment.validate(ctx); err != nil {
		return cognition.Transition{}, err
	}
	if err := ctx.Err(); err != nil {
		return cognition.Transition{}, err
	}
	scenario, err := environment.resolveScenario(ctx, reference)
	if err != nil {
		return cognition.Transition{}, err
	}
	candidate, err := environment.newKernel(scenario, environment.episode, historicalAuthorizer)
	if err != nil {
		return cognition.Transition{}, err
	}
	transition, err := candidate.Start(ctx, reference)
	if err != nil {
		_ = candidate.Close()
		return cognition.Transition{}, err
	}
	if err := candidate.Close(); err != nil {
		return cognition.Transition{}, fmt.Errorf("close Labyrinth start kernel: %w", err)
	}
	raw, digest, err := encodeExact(transition)
	if err != nil {
		return cognition.Transition{}, err
	}
	tx, err := environment.store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return cognition.Transition{}, err
	}
	defer tx.Rollback(ctx)
	result, err := tx.Exec(ctx, `
		INSERT INTO `+environment.store.relation("episodes")+`(
			episode_id,scenario_id,scenario_sha256,start_transition,start_transition_sha256,
			current_number,current_sha256,terminal
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (episode_id) DO NOTHING
	`, environment.episode.ID, reference.ID, reference.SHA256, raw, digest,
		int64(transition.Current.Number), transition.Current.SHA256, transition.Terminal)
	if err != nil {
		return cognition.Transition{}, fmt.Errorf("persist Labyrinth start receipt: %w", err)
	}
	if result.RowsAffected() == 0 {
		stored, loadErr := loadEpisodeRow(ctx, tx, environment.store.schema, environment.episode, true)
		if loadErr != nil {
			return cognition.Transition{}, loadErr
		}
		storedRaw, storedDigest, encodeErr := encodeExact(stored.Start)
		if encodeErr != nil {
			return cognition.Transition{}, encodeErr
		}
		if stored.Scenario != reference || storedDigest != digest || !bytes.Equal(storedRaw, raw) {
			return cognition.Transition{}, fmt.Errorf(
				"%w: episode %q is bound to another sealed start", ErrScenarioConflict, environment.episode.ID,
			)
		}
		transition = stored.Start.Clone()
	}
	if err := tx.Commit(ctx); err != nil {
		return cognition.Transition{}, fmt.Errorf("commit Labyrinth start receipt: %w", err)
	}
	return transition.Clone(), nil
}
