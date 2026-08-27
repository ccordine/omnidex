package roleplay

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func PrepareSimulationTurnTx(
	ctx context.Context,
	tx pgx.Tx,
	request SimulationTurnPreparationRequest,
) (SimulationTurnAuthority, error) {
	if ctx == nil || tx == nil {
		return SimulationTurnAuthority{}, fmt.Errorf("simulation turn preparation requires transaction authority")
	}
	if err := validateTurnPreparationRequest(request); err != nil {
		return SimulationTurnAuthority{}, err
	}
	worldID, sceneID, exactText, userTurn, err := loadPreparationMessageAuthorityTx(ctx, tx, request)
	if err != nil {
		return SimulationTurnAuthority{}, err
	}
	requestHash, err := simulationRequestHash("turn-preparation.v2", struct {
		SimulationTurnPreparationRequest
		ExactText string            `json:"exact_text"`
		UserTurn  UserTurnAuthority `json:"user_turn"`
	}{request, exactText, userTurn})
	if err != nil {
		return SimulationTurnAuthority{}, err
	}
	if authority, found, err := loadTurnPreparationTx(ctx, tx, request.OperationID, requestHash); err != nil || found {
		return authority, err
	}
	locked, err := lockSimulationSceneTx(ctx, tx, worldID, sceneID)
	if err != nil {
		return SimulationTurnAuthority{}, err
	}
	if authority, found, err := loadTurnPreparationTx(ctx, tx, request.OperationID, requestHash); err != nil || found {
		return authority, err
	}
	var action *SimulationAction
	switch request.InputKind {
	case SimulationTurnProse:
		if strings.HasPrefix(exactText, "/") {
			return SimulationTurnAuthority{}, fmt.Errorf("%w: slash input is not prose", ErrSimulationIllegal)
		}
	case SimulationTurnAction:
		if !strings.HasPrefix(exactText, "/") {
			return SimulationTurnAuthority{}, fmt.Errorf("%w: simulation action must begin with a slash", ErrSimulationIllegal)
		}
		parsed, parseErr := ParseSimulationAction(exactText)
		if parseErr != nil {
			return SimulationTurnAuthority{}, fmt.Errorf("%w: %v", ErrSimulationIllegal, parseErr)
		}
		action = &parsed
	case SimulationTurnExternalCommand:
		if !strings.HasPrefix(exactText, "/") {
			return SimulationTurnAuthority{}, fmt.Errorf("%w: external command must begin with a slash", ErrSimulationIllegal)
		}
	default:
		return SimulationTurnAuthority{}, fmt.Errorf("simulation turn input kind is invalid")
	}
	participantIDs := simulationParticipantIDs(locked.Participants)
	if err := validateUserTurnSceneAuthority(userTurn, participantIDs); err != nil {
		return SimulationTurnAuthority{}, err
	}
	responderIDs, err := simulationResponderIDs(
		participantIDs, locked.Sheet.ActiveCharacterID, userTurn,
	)
	if err != nil {
		return SimulationTurnAuthority{}, err
	}
	if len(responderIDs) == 0 {
		return SimulationTurnAuthority{}, fmt.Errorf("%w: enable at least one character other than the acting persona", ErrSimulationNotConfigured)
	}
	transition, responders, err := previewSimulationTurnTx(
		ctx, tx, locked, request.OperationID, requestHash, exactActionText(action, exactText), action,
		responderIDs,
	)
	if err != nil {
		return SimulationTurnAuthority{}, err
	}
	revision := locked.Sheet.Revision
	if transition != nil {
		revision = transition.AfterRevision
	}
	primary := responders[0]
	routes := make([]SimulationResponderRoute, len(responders))
	for index, responder := range responders {
		routes[index] = SimulationResponderRoute{
			Position: responder.Position, CharacterID: responder.CharacterID,
			GenerationConfig:     responder.GenerationConfig,
			NarrativeFingerprint: responder.NarrativeFingerprint,
		}
	}
	authority := SimulationTurnAuthority{
		PreparationID: request.OperationID, ChannelID: request.ChannelID,
		UserMessageID: request.UserMessageID, WorldID: worldID, SceneID: sceneID,
		BaseSceneRevision: locked.Sheet.Revision, SceneRevision: revision,
		ActiveCharacterID: locked.Sheet.ActiveCharacterID,
		UserTurn:          userTurn,
		InputKind:         request.InputKind,
		ExplicitAction:    action != nil, PendingTransition: transition,
		ParticipantCharacterIDs: participantIDs,
		GenerationConfig:        primary.GenerationConfig,
		NarrativeProjection:     primary.NarrativeProjection, NarrativeAuthority: primary.NarrativeAuthority,
		NarrativeFingerprint: primary.NarrativeFingerprint,
		Responders:           responders,
		ResponderRoutes:      routes,
		CreatedAt:            time.Now().UTC().Truncate(time.Microsecond),
	}
	if err := authority.Validate(); err != nil {
		return SimulationTurnAuthority{}, err
	}
	if err := persistTurnPreparationTx(ctx, tx, requestHash, authority); err != nil {
		return SimulationTurnAuthority{}, err
	}
	return authority, nil
}

func simulationResponderIDs(
	participantIDs []string,
	activeCharacterID string,
	userTurn UserTurnAuthority,
) ([]string, error) {
	responders := make([]string, 0, len(participantIDs))
	seen := make(map[string]struct{}, len(participantIDs))
	activeIndex := -1
	for index, id := range participantIDs {
		if validateIdentity(id, characterIdentity) != nil {
			return nil, fmt.Errorf("simulation response order contains an invalid participant")
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, fmt.Errorf("simulation response order contains a duplicated participant")
		}
		seen[id] = struct{}{}
		if id == activeCharacterID {
			activeIndex = index
		}
	}
	if activeIndex < 0 {
		return nil, fmt.Errorf("%w: initiative cursor is not a scene participant", ErrSimulationNotConfigured)
	}
	if userTurn.IsCharacter() {
		if _, present := seen[userTurn.CharacterID]; !present {
			return nil, fmt.Errorf(
				"%w: acting character is not a scene participant", ErrSimulationIllegal,
			)
		}
	}
	for offset := 0; offset < len(participantIDs); offset++ {
		id := participantIDs[(activeIndex+offset)%len(participantIDs)]
		if userTurn.IsCharacter() && id == userTurn.CharacterID {
			continue
		}
		responders = append(responders, id)
	}
	if len(responders) == 0 {
		return nil, fmt.Errorf(
			"%w: no responder remains after excluding the acting character", ErrSimulationNotConfigured,
		)
	}
	return responders, nil
}

func BindSimulationPreparationJobTx(
	ctx context.Context,
	tx pgx.Tx,
	preparationID string,
	jobID int64,
) error {
	if ctx == nil || tx == nil {
		return fmt.Errorf("simulation preparation binding requires transaction authority")
	}
	if err := validateIdentity(preparationID, transitionIdentity); err != nil {
		return err
	}
	if jobID < 1 {
		return fmt.Errorf("simulation preparation binding requires a positive job ID")
	}
	var existing int64
	err := tx.QueryRow(ctx, `
		SELECT job_id FROM roleplay_simulation_preparation_jobs
		WHERE preparation_id=$1
	`, preparationID).Scan(&existing)
	if err == nil {
		if existing != jobID {
			return fmt.Errorf("%w: preparation is bound to another job", ErrSimulationConflict)
		}
		return nil
	}
	if err != pgx.ErrNoRows {
		return err
	}
	command, err := tx.Exec(ctx, `
		INSERT INTO roleplay_simulation_preparation_jobs (preparation_id,job_id)
		SELECT preparation.operation_id,job.id
		FROM roleplay_simulation_turn_preparations AS preparation
		JOIN ai_channel_messages AS message ON message.id=preparation.user_message_id
		JOIN jobs AS job ON job.id=$2
		WHERE preparation.operation_id=$1
		  AND job.pipeline='chat' AND job.instruction=message.content
		  AND job.metadata->>'channel_id'=preparation.channel_id
		  AND job.metadata->>'channel_user_message_id'=preparation.user_message_id::text
		  AND job.metadata->>'roleplay_simulation_preparation_id'=preparation.operation_id
		  AND job.metadata->>'roleplay_world_id'=preparation.world_id
		  AND job.metadata->>'roleplay_scene_id'=preparation.scene_id
		  AND job.metadata->>'roleplay_scene_revision'=preparation.scene_revision::text
		  AND job.metadata->>'roleplay_input_kind'=preparation.input_kind
		  AND job.metadata->>'roleplay_narrative_fingerprint'=preparation.result->>'narrative_fingerprint'
		  AND job.metadata->>'roleplay_viewpoint_character_id'=
		      preparation.result->'responder_routes'->0->>'character_id'
		  AND job.metadata->'roleplay_participant_character_ids'=preparation.result->'participant_character_ids'
		  AND job.metadata->'roleplay_generation_config'=preparation.result->'generation_config'
		  AND job.metadata->'roleplay_responders'=preparation.result->'responder_routes'
		  AND job.metadata->'roleplay_user_turn'=preparation.result->'user_turn'
	`, preparationID, jobID)
	if err != nil {
		return simulationDefinitionError("simulation preparation job binding", err)
	}
	if command.RowsAffected() != 1 {
		return fmt.Errorf("%w: job does not match preparation authority", ErrSimulationIllegal)
	}
	return nil
}

func loadPreparationMessageAuthorityTx(
	ctx context.Context,
	tx pgx.Tx,
	request SimulationTurnPreparationRequest,
) (string, string, string, UserTurnAuthority, error) {
	var sceneID, exactText string
	userTurn, worldID, err := loadUserTurnAuthorityTx(ctx, tx, request.ChannelID, request.UserMessageID)
	if err != nil {
		return "", "", "", UserTurnAuthority{}, err
	}
	err = tx.QueryRow(ctx, `
		SELECT scene.id,message.content
		FROM ai_channels AS channel
		JOIN roleplay_worlds AS world ON world.channel_id=channel.id
		JOIN roleplay_current_scenes AS scene ON scene.world_id=world.id
		JOIN ai_channel_messages AS message ON message.channel_id=channel.id
		WHERE channel.id=$1 AND channel.mode='roleplay'
		  AND message.id=$2 AND message.role='user'
	`, request.ChannelID, request.UserMessageID).Scan(&sceneID, &exactText)
	if err == pgx.ErrNoRows {
		return "", "", "", UserTurnAuthority{}, fmt.Errorf("%w: channel message has no simulation scene", ErrSimulationNotConfigured)
	}
	if err != nil {
		return "", "", "", UserTurnAuthority{}, err
	}
	if exactText != userTurn.ExactText {
		return "", "", "", UserTurnAuthority{}, fmt.Errorf("roleplay user turn differs from exact message authority")
	}
	return worldID, sceneID, exactText, userTurn, nil
}

func validateTurnPreparationRequest(request SimulationTurnPreparationRequest) error {
	if err := validateIdentity(request.OperationID, transitionIdentity); err != nil {
		return err
	}
	if err := validateChannelID(request.ChannelID); err != nil {
		return err
	}
	if request.UserMessageID < 1 {
		return fmt.Errorf("simulation preparation requires a positive user message ID")
	}
	if request.InputKind != SimulationTurnProse && request.InputKind != SimulationTurnAction &&
		request.InputKind != SimulationTurnExternalCommand {
		return fmt.Errorf("simulation preparation input kind is invalid")
	}
	return nil
}

func exactActionText(action *SimulationAction, exactText string) string {
	if action == nil {
		return ""
	}
	return exactText
}

func simulationParticipantIDs(participants []SceneParticipantProjection) []string {
	ids := make([]string, len(participants))
	for index, participant := range participants {
		ids[index] = participant.CharacterID
	}
	return ids
}

func persistTurnPreparationTx(
	ctx context.Context,
	tx pgx.Tx,
	requestHash string,
	authority SimulationTurnAuthority,
) error {
	payload, err := json.Marshal(authority)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO roleplay_simulation_turn_preparations (
			operation_id,channel_id,user_message_id,world_id,scene_id,
			request_sha256,base_scene_revision,scene_revision,active_character_id,input_kind,explicit_action,
			pending_transition_id,result,created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13::jsonb,$14)
	`, authority.PreparationID, authority.ChannelID, authority.UserMessageID,
		authority.WorldID, authority.SceneID, requestHash, authority.BaseSceneRevision,
		authority.SceneRevision, authority.ActiveCharacterID, authority.InputKind, authority.ExplicitAction,
		pendingTransitionID(authority.PendingTransition), string(payload), authority.CreatedAt)
	return simulationDefinitionError("simulation turn preparation", err)
}

func pendingTransitionID(transition *SimulationTransitionResult) any {
	if transition == nil {
		return nil
	}
	return transition.OperationID
}
