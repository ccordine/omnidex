package queue

import (
	"context"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

type cognitionProgressAuthority struct {
	Actor      model.StepAttemptAuthority
	Header     taskLedgerHeader
	Episode    CognitionEpisode
	Graph      CognitionObligationGraphRecord
	Prepared   cognitionruntime.PreparedSnapshot
	Descriptor cognitionObligationDescriptor
}

func lockCognitionProgressAuthorityTx(
	ctx context.Context,
	tx pgx.Tx,
	command cognitionruntime.CompletionCommand,
	kind CognitionObligationCommandKind,
) (cognitionProgressAuthority, *cognitionruntime.EpisodeProgress, error) {
	var value cognitionProgressAuthority
	if err := command.Binding.Validate(); err != nil {
		return value, nil, err
	}
	if command.GraphVersion == 0 || command.SnapshotSHA256 == "" {
		return value, nil, fmt.Errorf("%w: cognition completion command identity is incomplete", ErrCognitionConflict)
	}
	if err := command.ObligationGraph.Validate(); err != nil {
		return value, nil, err
	}
	if err := command.Result.Validate(); err != nil {
		return value, nil, err
	}
	descriptor, err := describeCognitionRuntimeProgress(kind, command)
	if err != nil {
		return value, nil, err
	}
	value.Descriptor = descriptor
	value.Actor, err = cognitionRuntimeAuthority(command.Binding)
	if err != nil {
		return value, nil, err
	}
	if _, status, _, err := requireActiveStepAttemptTx(ctx, tx, value.Actor); err != nil {
		return value, nil, err
	} else if status != model.StepStatusRunning {
		return value, nil, staleStepAttemptError(value.Actor, "cognition progress actor is not running", nil)
	}
	value.Header, err = loadTaskLedgerHeaderTx(ctx, tx, value.Actor.JobID, true)
	if err != nil {
		return value, nil, err
	}
	value.Episode, _, err = loadCognitionEpisodeTx(ctx, tx, command.Binding.Episode.ID, true)
	if err != nil {
		return value, nil, err
	}
	if value.Episode.EpisodeID == "" {
		return value, nil, fmt.Errorf("%w: %s", ErrCognitionEpisodeNotFound, command.Binding.Episode.ID)
	}
	if err := cognitionAuthorityMatches(value.Actor, value.Episode); err != nil {
		return value, nil, err
	}
	if replay, found, err := loadCognitionRuntimeProgressReplayTx(ctx, tx, descriptor, command); err != nil {
		return value, nil, err
	} else if found {
		return value, &replay, nil
	}
	if value.Episode.Status != CognitionEpisodeActive {
		return value, nil, ErrCognitionTerminal
	}
	if err := requireNoUnresolvedCognitionActionTx(ctx, tx, value.Episode.EpisodeID); err != nil {
		return value, nil, err
	}
	value.Graph, _, err = loadCurrentCognitionObligationGraphTx(ctx, tx, value.Episode.EpisodeID, true)
	if err != nil {
		return value, nil, err
	}
	if value.Graph.EpisodeID == "" || value.Graph.Version != command.GraphVersion ||
		!reflect.DeepEqual(value.Graph.Graph, command.ObligationGraph) {
		return value, nil, fmt.Errorf("%w: completion command graph is not current", ErrCognitionConflict)
	}
	record, err := loadCognitionPreparedSnapshotBySHATx(
		ctx, tx, value.Actor, value.Episode, value.Graph, command.SnapshotSHA256,
	)
	if err != nil {
		return value, nil, err
	}
	value.Prepared = record.Prepared
	if err := value.Prepared.ValidateFor(command.Binding); err != nil {
		return value, nil, err
	}
	if value.Prepared.EnvironmentTerminal != command.EnvironmentTerminal ||
		value.Prepared.PublicOutcome != command.PublicOutcome ||
		!reflect.DeepEqual(value.Prepared.CompletionEvidenceRefs, command.CompletionEvidenceRefs) ||
		value.Prepared.Snapshot.CurrentRevision() != command.Result.Revision ||
		value.Prepared.Snapshot.CurrentObligation().ID != command.Result.ObligationID {
		return value, nil, fmt.Errorf("%w: completion command differs from prepared authority", ErrCognitionConflict)
	}
	current := value.Prepared.Snapshot.CurrentObligation()
	if err := command.Result.ValidateFor(
		current, value.Episode.CurrentRevision, value.Prepared.CompletionEvidenceRefs,
	); err != nil {
		return value, nil, err
	}
	if err := requireCognitionEvidenceRefsTx(
		ctx, tx, value.Episode.EpisodeID, value.Episode.CurrentRevision, command.Result.EvidenceRefs,
	); err != nil {
		return value, nil, err
	}
	return value, nil, nil
}

func cognitionProgress(
	episode CognitionEpisode,
	graph CognitionObligationGraphRecord,
	state cognitionruntime.EpisodeProgressState,
	completion *cognition.CompletionResult,
	publicOutcome string,
) cognitionruntime.EpisodeProgress {
	return cognitionruntime.EpisodeProgress{
		Episode: cognition.EpisodeRef{ID: episode.EpisodeID}, State: state,
		Revision: episode.CurrentRevision, GraphVersion: graph.Version,
		ObligationGraph: graph.Graph.Clone(), Completion: completion, PublicOutcome: publicOutcome,
	}
}
