package queue

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionstate"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) IngestCognitionTransition(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	actionID cognition.ActionID,
	transition cognition.Transition,
	facts cognitionstate.FactAcceptanceAuthority,
) (CognitionActionRecord, error) {
	if ctx == nil || r == nil || r.pool == nil || actionID == "" {
		return CognitionActionRecord{}, fmt.Errorf("cognition transition ingestion requires PostgreSQL, context, and action ID")
	}
	if err := validateStepAttemptAuthority(authority); err != nil {
		return CognitionActionRecord{}, err
	}
	if err := facts.Validate(); err != nil {
		return CognitionActionRecord{}, fmt.Errorf("cognition transition fact authority: %w", err)
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return CognitionActionRecord{}, err
	}
	defer tx.Rollback(ctx)
	if _, status, _, err := requireActiveStepAttemptTx(ctx, tx, authority); err != nil {
		return CognitionActionRecord{}, err
	} else if status != model.StepStatusRunning {
		return CognitionActionRecord{}, staleStepAttemptError(authority, "cognition transition actor is not running", nil)
	}
	var episodeID cognition.EpisodeID
	if err := tx.QueryRow(ctx, `SELECT episode_id FROM cognition_actions WHERE action_id=$1`, actionID).Scan(&episodeID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CognitionActionRecord{}, fmt.Errorf("%w: %s", ErrCognitionActionNotFound, actionID)
		}
		return CognitionActionRecord{}, err
	}
	header, err := loadTaskLedgerHeaderTx(ctx, tx, authority.JobID, true)
	if err != nil {
		return CognitionActionRecord{}, err
	}
	episode, found, err := loadCognitionEpisodeTx(ctx, tx, episodeID, true)
	if err != nil || !found {
		return CognitionActionRecord{}, fmt.Errorf("load cognition transition episode: %w", err)
	}
	if !reflect.DeepEqual(episode.FactAuthority, facts.Reference()) {
		return CognitionActionRecord{}, fmt.Errorf("%w: cognition fact authority changed", ErrCognitionConflict)
	}
	record, found, err := loadCognitionActionTx(ctx, tx, actionID, true)
	if err != nil {
		return CognitionActionRecord{}, err
	}
	if !found {
		return CognitionActionRecord{}, fmt.Errorf("%w: %s", ErrCognitionActionNotFound, actionID)
	}
	if authority.JobID != record.Origin.JobID || authority.Generation != record.Origin.Generation ||
		authority.StepID != record.Origin.StepID {
		return CognitionActionRecord{}, staleStepAttemptError(authority, "cognition action belongs to another step", nil)
	}
	if record.Status == CognitionActionSucceeded {
		if err := requireExactCognitionTransitionReplayTx(ctx, tx, record, transition); err != nil {
			return CognitionActionRecord{}, err
		}
		if err := requireCognitionAcceptedFactMaterializationReplayTx(
			ctx, tx, record.EpisodeID, transition, facts,
		); err != nil {
			return CognitionActionRecord{}, err
		}
		proof, err := cognitionTransitionProof(transition)
		if err != nil {
			return CognitionActionRecord{}, err
		}
		outcome := cognitionProposalAcceptedMaterialization
		if transition.Terminal {
			outcome = cognitionProposalRejectedTerminal
		}
		if err := requireCognitionProposalDispositionReplayTx(ctx, tx, record, outcome, proof); err != nil {
			return CognitionActionRecord{}, err
		}
		return record, nil
	}
	if record.Status != CognitionActionDispatched {
		return CognitionActionRecord{}, fmt.Errorf(
			"%w: action %q cannot ingest a transition from %q", ErrCognitionConflict, actionID, record.Status,
		)
	}
	if episode.Status != CognitionEpisodeActive || episode.CurrentRevision != record.ExpectedRevision {
		return CognitionActionRecord{}, fmt.Errorf("%w: cognition episode revision advanced before ingestion", ErrCognitionConflict)
	}
	episodeRef, err := cognition.NewEpisodeRef(record.EpisodeID)
	if err != nil {
		return CognitionActionRecord{}, err
	}
	if err := transition.ValidateApply(episodeRef, record.ExpectedRevision, record.Action); err != nil {
		return CognitionActionRecord{}, err
	}
	if err := insertCognitionTransitionTx(ctx, tx, authority, record.EpisodeID, transition); err != nil {
		return CognitionActionRecord{}, err
	}
	header, err = persistCognitionObservationsTx(ctx, tx, header, episode, record, transition)
	if err != nil {
		return CognitionActionRecord{}, err
	}
	header, err = persistCognitionTransitionFactsTx(
		ctx, tx, header, authority, episode.EpisodeID, record.ObligationID, transition, facts,
	)
	if err != nil {
		return CognitionActionRecord{}, err
	}
	header, err = applyCognitionObligationMaterializationTx(
		ctx, tx, header, episode, record, transition, authority,
	)
	if err != nil {
		return CognitionActionRecord{}, err
	}
	result, err := tx.Exec(ctx, `
		UPDATE cognition_actions SET status='succeeded',result_revision=$2,
		       result_revision_sha256=$3,resolved_at=clock_timestamp()
		WHERE action_id=$1 AND status='dispatched'
	`, actionID, int64(transition.Current.Number), transition.Current.SHA256)
	if err != nil {
		return CognitionActionRecord{}, err
	}
	if result.RowsAffected() != 1 {
		return CognitionActionRecord{}, fmt.Errorf("%w: cognition action changed before success", ErrCognitionConflict)
	}
	if err := insertCognitionActionEventTx(ctx, tx, actionID, authority, CognitionActionSucceeded, transition); err != nil {
		return CognitionActionRecord{}, err
	}
	result, err = tx.Exec(ctx, `
		UPDATE cognition_episodes
		SET current_revision=$2,current_revision_sha256=$3,
		    action_count=action_count+1,total_cost=total_cost+$4,version=version+1,
		    updated_at=clock_timestamp()
		WHERE episode_id=$1 AND status='active' AND current_revision=$5
	`, record.EpisodeID, int64(transition.Current.Number), transition.Current.SHA256,
		transition.Cost, int64(record.ExpectedRevision.Number))
	if err != nil {
		return CognitionActionRecord{}, err
	}
	if result.RowsAffected() != 1 {
		return CognitionActionRecord{}, fmt.Errorf("%w: cognition episode changed before transition", ErrCognitionConflict)
	}
	record, found, err = loadCognitionActionTx(ctx, tx, actionID, false)
	if err != nil || !found {
		return CognitionActionRecord{}, fmt.Errorf("reload succeeded cognition action: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return CognitionActionRecord{}, err
	}
	return record, nil
}

func requireExactCognitionTransitionReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	record CognitionActionRecord,
	transition cognition.Transition,
) error {
	raw, digest, err := cognitionJSON(transition)
	if err != nil {
		return err
	}
	var persistedRaw, persistedDigest string
	if err := tx.QueryRow(ctx, `
		SELECT transition_json,transition_sha256 FROM cognition_transitions
		WHERE episode_id=$1 AND action_id=$2
	`, record.EpisodeID, record.Action.ID).Scan(&persistedRaw, &persistedDigest); err != nil {
		return fmt.Errorf("load cognition transition replay: %w", err)
	}
	if persistedDigest != digest || persistedRaw != string(raw) {
		return fmt.Errorf("%w: cognition transition replay changed content", ErrCognitionConflict)
	}
	return nil
}
