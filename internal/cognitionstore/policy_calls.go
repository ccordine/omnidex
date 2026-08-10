package cognitionstore

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/queue"
)

func (store *Store) Start(
	ctx context.Context,
	attempt cognitionpolicy.CallAttempt,
) (cognitionpolicy.CallReservation, error) {
	if store == nil || store.repository == nil {
		return cognitionpolicy.CallReservation{}, fmt.Errorf("cognition call journal is uninitialized")
	}
	return (queue.CognitionPolicyCallJournal{Repository: store.repository}).Start(ctx, attempt)
}

func (store *Store) Finish(
	ctx context.Context,
	attempt cognitionpolicy.CallAttempt,
	result cognitionpolicy.CallResult,
) error {
	if store == nil || store.repository == nil {
		return fmt.Errorf("cognition call journal is uninitialized")
	}
	return (queue.CognitionPolicyCallJournal{Repository: store.repository}).Finish(ctx, attempt, result)
}
