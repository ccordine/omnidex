package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

const maxChannelSessionStateRetries = 4

// SubmitChannelSessionTurn is the sole free-form interactive turn boundary.
// The persisted server state selects enqueue, replan, or feedback atomically.
func (r *Repository) SubmitChannelSessionTurn(
	ctx context.Context,
	command ChannelSessionTurnCommand,
) (ChannelSessionTurnResult, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return ChannelSessionTurnResult{}, fmt.Errorf(
			"channel session turn requires PostgreSQL and context",
		)
	}
	command, err := normalizeChannelSessionTurnCommand(command)
	if err != nil {
		return ChannelSessionTurnResult{}, err
	}
	descriptor, err := describeLifecycleOperation(
		command.OperationID,
		LifecycleChannelSession,
		command,
	)
	if err != nil {
		return ChannelSessionTurnResult{}, err
	}
	for attempt := 0; attempt < maxChannelSessionStateRetries; attempt++ {
		result, retry, err := r.submitChannelSessionTurnAttempt(
			ctx,
			command,
			descriptor,
		)
		if err != nil {
			return ChannelSessionTurnResult{}, err
		}
		if !retry {
			return result, nil
		}
	}
	return ChannelSessionTurnResult{}, fmt.Errorf(
		"%w: channel %q active job changed during %d consecutive turn attempts",
		ErrStaleJobGeneration,
		command.ChannelID,
		maxChannelSessionStateRetries,
	)
}

func (r *Repository) submitChannelSessionTurnAttempt(
	ctx context.Context,
	command ChannelSessionTurnCommand,
	descriptor lifecycleOperationDescriptor,
) (ChannelSessionTurnResult, bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ChannelSessionTurnResult{}, false, fmt.Errorf(
			"begin channel %q session turn: %w",
			command.ChannelID,
			err,
		)
	}
	defer tx.Rollback(ctx)

	if err := lockLifecycleOperationIdentityTx(ctx, tx, command.OperationID); err != nil {
		return ChannelSessionTurnResult{}, false, err
	}
	identityCreated, err := reserveLifecycleOperationIdentityTx(
		ctx,
		tx,
		descriptor.ID,
		descriptor.Kind,
		descriptor.SHA256,
		descriptor.Payload,
	)
	if err != nil {
		return ChannelSessionTurnResult{}, false, err
	}
	operationExists, err := channelSessionTurnOperationExistsTx(ctx, tx, command.OperationID)
	if err != nil {
		return ChannelSessionTurnResult{}, false, err
	}
	if operationExists {
		if identityCreated {
			return ChannelSessionTurnResult{}, false, fmt.Errorf(
				"channel session operation %q appeared after creating its identity",
				command.OperationID,
			)
		}
		authority, err := lockChannelTurnAuthorityTx(ctx, tx, command.ChannelID)
		if err != nil {
			return ChannelSessionTurnResult{}, false, err
		}
		if err := validateAssistantSessionAuthority(command, authority); err != nil {
			return ChannelSessionTurnResult{}, false, err
		}
		replay, found, err := loadChannelSessionTurnOperationTx(
			ctx,
			tx,
			command,
			authority,
		)
		if err != nil {
			return ChannelSessionTurnResult{}, false, err
		}
		if !found {
			return ChannelSessionTurnResult{}, false, fmt.Errorf(
				"channel session operation %q disappeared during replay",
				command.OperationID,
			)
		}
		if err := tx.Commit(ctx); err != nil {
			return ChannelSessionTurnResult{}, false, err
		}
		return replay, false, nil
	}
	if !identityCreated {
		return ChannelSessionTurnResult{}, false, fmt.Errorf(
			"channel session operation %q is registered without its immutable result",
			command.OperationID,
		)
	}

	candidate, candidateFound, err := activeChannelJobTx(ctx, tx, command.ChannelID)
	if err != nil {
		return ChannelSessionTurnResult{}, false, err
	}
	if candidateFound {
		candidate, err = lockedJobTx(ctx, tx, candidate.ID)
		if err != nil {
			return ChannelSessionTurnResult{}, false, err
		}
		if terminalJobStatus(candidate.Status) {
			candidate, candidateFound = model.Job{}, false
		}
	}
	authority, err := lockChannelTurnAuthorityTx(ctx, tx, command.ChannelID)
	if err != nil {
		return ChannelSessionTurnResult{}, false, err
	}
	if err := validateAssistantSessionAuthority(command, authority); err != nil {
		return ChannelSessionTurnResult{}, false, err
	}
	current, currentFound, err := activeChannelJobTx(ctx, tx, command.ChannelID)
	if err != nil {
		return ChannelSessionTurnResult{}, false, err
	}
	if currentFound != candidateFound || currentFound && current.ID != candidate.ID {
		return ChannelSessionTurnResult{}, true, nil
	}
	result, stepID, err := r.applyChannelSessionTurnTx(
		ctx,
		tx,
		command,
		authority,
		candidate,
		candidateFound,
	)
	if err != nil {
		return ChannelSessionTurnResult{}, false, err
	}
	if err := insertChannelSessionTurnOperationTx(
		ctx,
		tx,
		descriptor,
		result,
		stepID,
	); err != nil {
		return ChannelSessionTurnResult{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ChannelSessionTurnResult{}, false, fmt.Errorf(
			"commit channel %q session turn: %w",
			command.ChannelID,
			err,
		)
	}
	return result, false, nil
}

func validateAssistantSessionAuthority(
	command ChannelSessionTurnCommand,
	authority lockedChannelTurnAuthority,
) error {
	if authority.Scope != model.ChannelScopeUser || authority.Mode != model.ChannelModeAssistant {
		return fmt.Errorf(
			"%w: channel %q is not an assistant conversation",
			ErrChannelSessionAuthority,
			command.ChannelID,
		)
	}
	if authority.WorkspaceRoot != command.WorkspaceRoot {
		return fmt.Errorf(
			"%w: channel %q is bound to %q, not %q",
			ErrChannelSessionWorkspace,
			command.ChannelID,
			authority.WorkspaceRoot,
			command.WorkspaceRoot,
		)
	}
	if err := requireCLIChatSessionWorkspaceBinding(
		command.ChannelID,
		command.WorkspaceRoot,
		command.WorkspaceIdentity,
	); err != nil {
		return err
	}
	return nil
}
