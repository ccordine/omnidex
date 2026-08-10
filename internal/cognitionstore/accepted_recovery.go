package cognitionstore

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionruntime"
)

func (store *Store) RecoverAccepted(
	ctx context.Context,
	binding cognitionruntime.Binding,
) (*cognitionruntime.AcceptedDecisionRecovery, error) {
	if store == nil || store.repository == nil {
		return nil, fmt.Errorf("accepted cognition recovery store is uninitialized")
	}
	return store.repository.RecoverAcceptedCognitionDecision(ctx, binding)
}
