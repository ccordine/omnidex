package roleplay

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// AppendOngoingActionResolutionTx records the semantic result for one exact
// response completion. A state row is appended only when the returned leaf
// differs byte-for-byte from the character's current leaf.
func AppendOngoingActionResolutionTx(
	ctx context.Context,
	tx pgx.Tx,
	completionOperationID string,
	responsePosition int,
	previousOngoingAction, ongoingAction *string,
) (OngoingActionResolution, error) {
	if ctx == nil || tx == nil {
		return OngoingActionResolution{}, fmt.Errorf(
			"roleplay ongoing-action resolution requires transaction authority",
		)
	}
	if err := validateOngoingActionResolutionInput(
		completionOperationID, OngoingActionSourceResponse, responsePosition,
		previousOngoingAction, ongoingAction,
	); err != nil {
		return OngoingActionResolution{}, err
	}
	completion, err := lockOngoingActionCompletionCharacterTx(
		ctx, tx, completionOperationID, responsePosition,
	)
	if err != nil {
		return OngoingActionResolution{}, err
	}
	return appendOngoingActionResolutionTx(
		ctx, tx, completion, previousOngoingAction, ongoingAction,
	)
}

// AppendUserOngoingActionResolutionTx records the one semantic action-state
// result for the exact typed acting-character contribution bound to a prepared
// turn. Its source position is the durable actor sentinel, never a responder
// position.
func AppendUserOngoingActionResolutionTx(
	ctx context.Context,
	tx pgx.Tx,
	completionOperationID string,
	preparationID string,
	characterID string,
	previousOngoingAction, ongoingAction *string,
) (OngoingActionResolution, error) {
	if ctx == nil || tx == nil {
		return OngoingActionResolution{}, fmt.Errorf(
			"roleplay user ongoing-action resolution requires transaction authority",
		)
	}
	if err := validateOngoingActionResolutionInput(
		completionOperationID, OngoingActionSourceUserAction,
		UserActionOngoingActionSourcePosition, previousOngoingAction, ongoingAction,
	); err != nil {
		return OngoingActionResolution{}, err
	}
	if err := validateIdentity(preparationID, transitionIdentity); err != nil {
		return OngoingActionResolution{}, fmt.Errorf(
			"roleplay user ongoing-action preparation: %w", err,
		)
	}
	if err := validateIdentity(characterID, characterIdentity); err != nil {
		return OngoingActionResolution{}, fmt.Errorf(
			"roleplay user ongoing-action character: %w", err,
		)
	}
	source, err := lockOngoingActionUserCharacterTx(
		ctx, tx, completionOperationID, preparationID, characterID,
	)
	if err != nil {
		return OngoingActionResolution{}, err
	}
	return appendOngoingActionResolutionTx(
		ctx, tx, source, previousOngoingAction, ongoingAction,
	)
}

func RequireOngoingActionResolutionReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	completionOperationID string,
	responsePosition int,
	previousOngoingAction, ongoingAction *string,
) error {
	if ctx == nil || tx == nil {
		return fmt.Errorf("roleplay ongoing-action replay requires transaction authority")
	}
	if err := validateOngoingActionResolutionInput(
		completionOperationID, OngoingActionSourceResponse, responsePosition,
		previousOngoingAction, ongoingAction,
	); err != nil {
		return err
	}
	return requireOngoingActionResolutionReplayTx(
		ctx, tx, completionOperationID, OngoingActionSourceResponse, responsePosition,
		"", previousOngoingAction, ongoingAction,
	)
}

func RequireUserOngoingActionResolutionReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	completionOperationID string,
	characterID string,
	previousOngoingAction, ongoingAction *string,
) error {
	if ctx == nil || tx == nil {
		return fmt.Errorf("roleplay user ongoing-action replay requires transaction authority")
	}
	if err := validateOngoingActionResolutionInput(
		completionOperationID, OngoingActionSourceUserAction,
		UserActionOngoingActionSourcePosition, previousOngoingAction, ongoingAction,
	); err != nil {
		return err
	}
	if err := validateIdentity(characterID, characterIdentity); err != nil {
		return fmt.Errorf("roleplay user ongoing-action replay character: %w", err)
	}
	return requireOngoingActionResolutionReplayTx(
		ctx, tx, completionOperationID, OngoingActionSourceUserAction,
		UserActionOngoingActionSourcePosition, characterID,
		previousOngoingAction, ongoingAction,
	)
}

func requireOngoingActionResolutionReplayTx(
	ctx context.Context,
	tx pgx.Tx,
	completionOperationID string,
	sourceKind OngoingActionSourceKind,
	sourcePosition int,
	expectedCharacterID string,
	previousOngoingAction, ongoingAction *string,
) error {
	resolution, err := scanOngoingActionResolution(tx.QueryRow(ctx, `
		SELECT completion_operation_id,source_kind,source_position,world_id,character_id,
		       source_message_id,previous_state_id,current_state_id,
		       previous_action_text,action_text,changed,authority_namespace,created_at
		FROM roleplay_ongoing_action_resolutions
		WHERE completion_operation_id=$1 AND source_kind=$2 AND source_position=$3
	`, completionOperationID, sourceKind, sourcePosition))
	if err == pgx.ErrNoRows {
		return fmt.Errorf("roleplay ongoing-action replay resolution is absent")
	}
	if err != nil {
		return fmt.Errorf("load roleplay ongoing-action replay resolution: %w", err)
	}
	if (expectedCharacterID != "" && resolution.CharacterID != expectedCharacterID) ||
		!equalOptionalOngoingAction(
			resolution.PreviousOngoingAction, previousOngoingAction,
		) || !equalOptionalOngoingAction(resolution.OngoingAction, ongoingAction) {
		return fmt.Errorf(
			"roleplay ongoing-action replay differs from exact completion authority",
		)
	}
	return nil
}

type ongoingActionCompletion struct {
	operationID     string
	sourceKind      OngoingActionSourceKind
	sourcePosition  int
	worldID         string
	characterID     string
	sourceMessageID int64
}

func lockOngoingActionCompletionCharacterTx(
	ctx context.Context,
	tx pgx.Tx,
	completionOperationID string,
	responsePosition int,
) (ongoingActionCompletion, error) {
	var completion ongoingActionCompletion
	err := tx.QueryRow(ctx, `
		SELECT completion.operation_id,completion.response_position,
		       completion.world_id,completion.viewpoint_character_id,
		       completion.source_message_id
		FROM roleplay_turn_completions AS completion
		JOIN roleplay_characters AS character
		  ON character.world_id=completion.world_id
		 AND character.id=completion.viewpoint_character_id
		WHERE completion.operation_id=$1 AND completion.response_position=$2
		FOR UPDATE OF character
	`, completionOperationID, responsePosition).Scan(
		&completion.operationID, &completion.sourcePosition, &completion.worldID,
		&completion.characterID, &completion.sourceMessageID,
	)
	if err == pgx.ErrNoRows {
		return ongoingActionCompletion{}, fmt.Errorf(
			"roleplay ongoing-action completion authority is absent",
		)
	}
	if err != nil {
		return ongoingActionCompletion{}, fmt.Errorf(
			"lock roleplay ongoing-action character authority: %w", err,
		)
	}
	completion.sourceKind = OngoingActionSourceResponse
	return completion, nil
}

func lockOngoingActionUserCharacterTx(
	ctx context.Context,
	tx pgx.Tx,
	completionOperationID string,
	preparationID string,
	characterID string,
) (ongoingActionCompletion, error) {
	completion := ongoingActionCompletion{
		operationID:    completionOperationID,
		sourceKind:     OngoingActionSourceUserAction,
		sourcePosition: UserActionOngoingActionSourcePosition,
	}
	err := tx.QueryRow(ctx, `
		SELECT preparation.world_id,user_turn.persona_character_id,
		       user_turn.user_message_id
		FROM roleplay_simulation_turn_preparations AS preparation
		JOIN roleplay_simulation_preparation_jobs AS binding
		  ON binding.preparation_id=preparation.operation_id
		JOIN roleplay_user_turns AS user_turn
		  ON user_turn.user_message_id=preparation.user_message_id
		 AND user_turn.channel_id=preparation.channel_id
		 AND user_turn.world_id=preparation.world_id
		JOIN roleplay_characters AS character
		  ON character.world_id=preparation.world_id
		 AND character.id=user_turn.persona_character_id
		WHERE preparation.operation_id=$1
		  AND user_turn.persona_kind='character'
		  AND user_turn.persona_character_id=$2
		  AND preparation.result->'user_turn'=user_turn.authority
		  AND preparation.result->'participant_character_ids' ? user_turn.persona_character_id
		  AND EXISTS (
		      SELECT 1 FROM jsonb_array_elements(user_turn.parts) AS part(value)
		      WHERE part.value->>'kind'='action'
		  )
		FOR UPDATE OF character
	`, preparationID, characterID).Scan(
		&completion.worldID, &completion.characterID, &completion.sourceMessageID,
	)
	if err == pgx.ErrNoRows {
		return ongoingActionCompletion{}, fmt.Errorf(
			"roleplay user ongoing-action preparation authority is absent",
		)
	}
	if err != nil {
		return ongoingActionCompletion{}, fmt.Errorf(
			"lock roleplay user ongoing-action character authority: %w", err,
		)
	}
	return completion, nil
}

func appendOngoingActionResolutionTx(
	ctx context.Context,
	tx pgx.Tx,
	source ongoingActionCompletion,
	previousOngoingAction, ongoingAction *string,
) (OngoingActionResolution, error) {
	currentState, hasCurrentState, err := loadLatestOngoingActionStateTx(
		ctx, tx, source.worldID, source.characterID,
	)
	if err != nil {
		return OngoingActionResolution{}, err
	}
	var currentAction *string
	var previousStateID *string
	if hasCurrentState {
		currentAction = currentState.OngoingAction
		previousStateID = &currentState.ID
	}
	if !equalOptionalOngoingAction(currentAction, previousOngoingAction) {
		return OngoingActionResolution{}, fmt.Errorf(
			"roleplay ongoing-action previous leaf differs from exact character state",
		)
	}

	changed := !equalOptionalOngoingAction(previousOngoingAction, ongoingAction)
	currentStateID := cloneOptionalOngoingAction(previousStateID)
	if changed {
		state, appendErr := appendOngoingActionStateTx(ctx, tx, source, ongoingAction)
		if appendErr != nil {
			return OngoingActionResolution{}, appendErr
		}
		currentStateID = &state.ID
	}
	resolution, err := scanOngoingActionResolution(tx.QueryRow(ctx, `
		INSERT INTO roleplay_ongoing_action_resolutions (
			completion_operation_id,source_kind,source_position,world_id,character_id,
			source_message_id,previous_state_id,current_state_id,
			previous_action_text,action_text,changed
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING completion_operation_id,source_kind,source_position,world_id,character_id,
		          source_message_id,previous_state_id,current_state_id,
		          previous_action_text,action_text,changed,authority_namespace,created_at
	`, source.operationID, source.sourceKind, source.sourcePosition,
		source.worldID, source.characterID, source.sourceMessageID,
		previousStateID, currentStateID, previousOngoingAction, ongoingAction, changed))
	if err != nil {
		return OngoingActionResolution{}, fmt.Errorf(
			"append roleplay ongoing-action resolution: %w", err,
		)
	}
	return resolution, nil
}

func appendOngoingActionStateTx(
	ctx context.Context,
	tx pgx.Tx,
	completion ongoingActionCompletion,
	ongoingAction *string,
) (OngoingActionState, error) {
	stateID, err := newIdentity("rpo_")
	if err != nil {
		return OngoingActionState{}, err
	}
	state, err := scanOngoingActionState(tx.QueryRow(ctx, `
		INSERT INTO roleplay_ongoing_action_states (
			id,world_id,character_id,source_completion_operation_id,
			source_kind,source_position,source_message_id,action_text
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id,ordinal,world_id,character_id,source_completion_operation_id,
		          source_kind,source_position,source_message_id,action_text,
		          authority_namespace,created_at
	`, stateID, completion.worldID, completion.characterID, completion.operationID,
		completion.sourceKind, completion.sourcePosition, completion.sourceMessageID, ongoingAction))
	if err != nil {
		return OngoingActionState{}, fmt.Errorf("append roleplay ongoing-action state: %w", err)
	}
	return state, nil
}

func loadLatestOngoingActionStateTx(
	ctx context.Context,
	tx pgx.Tx,
	worldID, characterID string,
) (OngoingActionState, bool, error) {
	state, err := scanOngoingActionState(tx.QueryRow(ctx, `
		SELECT id,ordinal,world_id,character_id,source_completion_operation_id,
		       source_kind,source_position,source_message_id,action_text,
		       authority_namespace,created_at
		FROM roleplay_ongoing_action_states
		WHERE world_id=$1 AND character_id=$2
		ORDER BY ordinal DESC,id DESC LIMIT 1
	`, worldID, characterID))
	if err == pgx.ErrNoRows {
		return OngoingActionState{}, false, nil
	}
	if err != nil {
		return OngoingActionState{}, false, fmt.Errorf(
			"load current roleplay ongoing-action state: %w", err,
		)
	}
	return state, true, nil
}
