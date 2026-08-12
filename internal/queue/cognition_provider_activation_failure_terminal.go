package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func terminalizeCognitionProviderActivationFailureTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	episodeID cognition.EpisodeID,
	failureRecordID string,
) error {
	episode, found, err := loadCognitionEpisodeTx(ctx, tx, episodeID, true)
	if err != nil || !found {
		return err
	}
	if episode.Status != CognitionEpisodeActive {
		return fmt.Errorf(
			"%w: provider activation failed after episode %q became %s",
			ErrCognitionConflict, episodeID, episode.Status,
		)
	}
	binding, err := cognitionruntime.NewBinding(
		cognition.EpisodeRef{ID: episodeID}, cognitionAttempt(authority),
	)
	if err != nil {
		return err
	}
	evidence, err := cognitionruntime.NewProviderActivationCancellationEvidence(failureRecordID)
	if err != nil {
		return err
	}
	command := cognitionruntime.CancellationCommand{
		Binding: binding, ExpectedRevision: episode.CurrentRevision,
		Code: cognitionruntime.CancellationProviderActivation, SourceEvidence: evidence,
	}
	normalized, err := newCognitionCancellationCommand(command)
	if err != nil {
		return err
	}
	terminal, active, err := prepareCognitionCancellationTx(ctx, tx, normalized)
	if err != nil {
		return err
	}
	if !active {
		return fmt.Errorf("%w: provider activation failure lost its active episode", ErrCognitionConflict)
	}
	if err := insertCognitionCancellationTx(ctx, tx, normalized); err != nil {
		return err
	}
	seal, err := sealCognitionEpisodeTx(ctx, tx, terminal)
	if err != nil {
		return err
	}
	if seal.Outcome != CognitionEpisodeCanceled || seal.AuthorityKind != cognitionTerminalAuthorityWorker ||
		seal.SealedBy != authority {
		return fmt.Errorf("%w: provider activation failure terminal seal changed", ErrCognitionConflict)
	}
	return nil
}
