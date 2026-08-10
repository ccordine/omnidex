package queue

import (
	"context"
	"errors"

	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

const cognitionRuntimeProjectionWorkKind = "cognition_runtime_decision"

func storeLiveCognitionProjectionTx(
	ctx context.Context,
	tx pgx.Tx,
	authority model.StepAttemptAuthority,
	projection contextbuilder.Projection,
) (ContextProjectionRecord, error) {
	owner := ContextProjectionAuthority{
		StepAttemptAuthority: authority,
		WorkKind:             cognitionRuntimeProjectionWorkKind,
		Mode:                 ContextProjectionModeLive,
	}
	if existing, err := loadContextProjectionTx(ctx, tx, projection.ID); err == nil {
		return exactContextProjectionReplay(existing, owner, projection)
	} else if !errors.Is(err, ErrContextProjectionNotFound) {
		return ContextProjectionRecord{}, err
	}
	if err := validateCurrentContextProjectionAuthorityTx(ctx, tx, owner, projection); err != nil {
		return ContextProjectionRecord{}, err
	}
	record, inserted, err := insertContextProjectionTx(ctx, tx, owner, projection)
	if err != nil {
		return ContextProjectionRecord{}, err
	}
	if !inserted {
		existing, err := loadContextProjectionTx(ctx, tx, projection.ID)
		if err != nil {
			return ContextProjectionRecord{}, err
		}
		return exactContextProjectionReplay(existing, owner, projection)
	}
	if err := insertContextProjectionReferencesTx(ctx, tx, record); err != nil {
		return ContextProjectionRecord{}, err
	}
	return record, nil
}
