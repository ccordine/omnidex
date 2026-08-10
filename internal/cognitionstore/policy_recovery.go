package cognitionstore

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionruntime"
)

func (store *Store) ReplayTerminalPolicyOutcome(
	ctx context.Context,
	binding cognitionruntime.Binding,
) (bool, error) {
	if store == nil || store.repository == nil {
		return false, fmt.Errorf("cognition policy recovery store is uninitialized")
	}
	return store.repository.ReplayTerminalCognitionPolicyOutcome(ctx, binding)
}

func (store *Store) AbandonIndeterminate(
	ctx context.Context,
	binding cognitionruntime.Binding,
) (*cognitionruntime.PolicyCallAbandonment, error) {
	if store == nil || store.repository == nil {
		return nil, fmt.Errorf("cognition policy recovery store is uninitialized")
	}
	return store.repository.AbandonIndeterminateCognitionPolicyCall(ctx, binding)
}
