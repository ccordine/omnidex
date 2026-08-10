package queue

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func retireCognitionEpisodesForLifecycleTx(
	ctx context.Context,
	tx pgx.Tx,
	descriptor lifecycleOperationDescriptor,
	jobID, generation int64,
	stepIDs []int64,
) (cognitionLifecycleSealSet, error) {
	code, publicOutcome, err := lifecycleCancellationCode(descriptor.Kind)
	if err != nil {
		return cognitionLifecycleSealSet{}, err
	}
	if jobID <= 0 || generation <= 0 || !cognitionDigestPattern.MatchString(descriptor.SHA256) {
		return cognitionLifecycleSealSet{}, fmt.Errorf("cognition lifecycle retirement authority is incomplete")
	}
	if len(stepIDs) > 0 {
		if err := lockActiveStepAttemptsForLifecycleTx(ctx, tx, jobID, generation, stepIDs); err != nil {
			return cognitionLifecycleSealSet{}, err
		}
	}
	episodes, err := loadLifecycleCognitionScopeTx(ctx, tx, jobID, generation, stepIDs)
	if err != nil {
		return cognitionLifecycleSealSet{}, err
	}
	entries := make([]cognitionLifecycleSealEntry, 0, len(episodes))
	for _, episode := range episodes {
		entry, err := sealLifecycleCognitionEpisodeTx(
			ctx, tx, descriptor, episode, code, publicOutcome,
		)
		if err != nil {
			return cognitionLifecycleSealSet{}, err
		}
		entries = append(entries, entry)
	}
	set, err := newCognitionLifecycleSealSet(descriptor, jobID, generation, entries)
	if err != nil {
		return cognitionLifecycleSealSet{}, err
	}
	if err := insertCognitionLifecycleSealSetTx(ctx, tx, set); err != nil {
		return cognitionLifecycleSealSet{}, err
	}
	return set, nil
}

func requireCognitionLifecycleSealSetReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	descriptor lifecycleOperationDescriptor,
	jobID, generation int64,
) error {
	set, found, err := loadCognitionLifecycleSealSetTx(ctx, tx, descriptor.ID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w: lifecycle operation lacks cognition seal-set authority", ErrCognitionConflict)
	}
	if set.OperationID != descriptor.ID || set.OperationKind != descriptor.Kind ||
		set.OperationSHA256 != descriptor.SHA256 || set.JobID != jobID || set.Generation != generation {
		return fmt.Errorf("%w: lifecycle cognition seal-set replay changed", ErrLifecycleOperationConflict)
	}
	var active int
	if err := tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM cognition_episodes
		WHERE job_id=$1 AND generation=$2 AND status='active'
	`, jobID, generation).Scan(&active); err != nil {
		return err
	}
	if active != 0 {
		return fmt.Errorf("%w: lifecycle replay retains active cognition episodes", ErrCognitionConflict)
	}
	for _, entry := range set.Entries {
		seal, err := loadCognitionTerminalSealTx(ctx, tx, entry.EpisodeID)
		if err != nil {
			return err
		}
		if seal.AuthorityKind != cognitionTerminalAuthorityLifecycle ||
			seal.LifecycleOperationID != descriptor.ID || seal.TraceSHA256 != entry.TraceSHA256 {
			return fmt.Errorf("%w: lifecycle replay seal changed", ErrCognitionConflict)
		}
	}
	return nil
}
