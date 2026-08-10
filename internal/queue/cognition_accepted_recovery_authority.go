package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func loadExactAcceptedCognitionRecoveryTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	episode CognitionEpisode,
	graph CognitionObligationGraphRecord,
	binding cognitionruntime.Binding,
	ref *cognitionruntime.AcceptedDecisionRecoveryRef,
) (cognitionruntime.AcceptedDecisionRecovery, error) {
	if ref == nil {
		return cognitionruntime.AcceptedDecisionRecovery{}, fmt.Errorf(
			"%w: accepted recovery receipt is required", ErrCognitionConflict,
		)
	}
	if err := ref.Validate(); err != nil {
		return cognitionruntime.AcceptedDecisionRecovery{}, err
	}
	stored, found, err := loadAcceptedCognitionRecoveryTx(ctx, tx, binding, ref.PolicyCallID, true)
	if err != nil || !found {
		return cognitionruntime.AcceptedDecisionRecovery{}, fmt.Errorf(
			"%w: accepted recovery receipt is not durable: %v", ErrCognitionConflict, err,
		)
	}
	if stored.ID != ref.ID || stored.SHA256 != ref.SHA256 ||
		stored.SourcePolicyCallID != ref.PolicyCallID {
		return cognitionruntime.AcceptedDecisionRecovery{}, fmt.Errorf(
			"%w: accepted recovery receipt changed identity", ErrCognitionConflict,
		)
	}
	recovery, err := buildAcceptedCognitionRecoveryTx(
		ctx, tx, binding, authority, episode, graph, ref.PolicyCallID,
	)
	if err != nil {
		return cognitionruntime.AcceptedDecisionRecovery{}, err
	}
	if recovery.Ref() != *ref {
		return cognitionruntime.AcceptedDecisionRecovery{}, fmt.Errorf(
			"%w: accepted recovery receipt does not bind reconstructed authority", ErrCognitionConflict,
		)
	}
	if err := validateAcceptedCognitionRecoveryReplay(stored, recovery); err != nil {
		return cognitionruntime.AcceptedDecisionRecovery{}, err
	}
	return recovery, nil
}

func preparedSnapshotForRecoveryTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	episode CognitionEpisode,
	graph CognitionObligationGraphRecord,
	binding cognitionruntime.Binding,
	ref *cognitionruntime.AcceptedDecisionRecoveryRef,
) (CognitionRuntimeSnapshotRecord, cognitionruntime.AcceptedDecisionRecovery, error) {
	recovery, err := loadExactAcceptedCognitionRecoveryTx(
		ctx, tx, authority, episode, graph, binding, ref,
	)
	if err != nil {
		return CognitionRuntimeSnapshotRecord{}, cognitionruntime.AcceptedDecisionRecovery{}, err
	}
	return CognitionRuntimeSnapshotRecord{Prepared: recovery.Prepared}, recovery, nil
}
