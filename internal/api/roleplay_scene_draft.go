package api

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

const roleplaySceneDraftSchema = "roleplay_scene_draft_v1"

type roleplaySceneDraftParticipant struct {
	CharacterID string `json:"character_id"`
}

type roleplaySceneDraft struct {
	Schema        string                          `json:"schema"`
	ChannelID     model.ChannelID                 `json:"channel_id"`
	WorldID       string                          `json:"world_id"`
	Revision      int64                           `json:"revision"`
	SceneRevision int64                           `json:"scene_revision"`
	Participants  []roleplaySceneDraftParticipant `json:"participants"`
}

func emptyRoleplaySceneDraft(channelID model.ChannelID, worldID string) roleplaySceneDraft {
	return roleplaySceneDraft{
		Schema: roleplaySceneDraftSchema, ChannelID: channelID, WorldID: worldID,
		Revision: 0, SceneRevision: 0, Participants: []roleplaySceneDraftParticipant{},
	}
}

func (s *Server) loadRoleplaySceneDraftLocked(
	ctx context.Context,
	sessionID string,
	channelID model.ChannelID,
	worldID string,
) (roleplaySceneDraft, error) {
	state, _, err := s.loadUIState(ctx, sessionID)
	if err != nil {
		return roleplaySceneDraft{}, err
	}
	raw, exists := state[roleplaySceneDraftStateKey(channelID)]
	if !exists {
		return emptyRoleplaySceneDraft(channelID, worldID), nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return roleplaySceneDraft{}, err
	}
	var draft roleplaySceneDraft
	if err := json.Unmarshal(encoded, &draft); err != nil {
		return roleplaySceneDraft{}, fmt.Errorf("persisted roleplay scene draft is invalid: %w", err)
	}
	if err := validateRoleplaySceneDraft(draft, channelID, worldID); err != nil {
		return roleplaySceneDraft{}, err
	}
	return draft, nil
}

func (s *Server) persistRoleplaySceneDraftLocked(
	ctx context.Context,
	sessionID string,
	draft roleplaySceneDraft,
) error {
	if err := validateRoleplaySceneDraft(draft, draft.ChannelID, draft.WorldID); err != nil {
		return err
	}
	state, _, err := s.loadUIState(ctx, sessionID)
	if err != nil {
		return err
	}
	state[roleplaySceneDraftStateKey(draft.ChannelID)] = draft
	_, err = s.persistUIState(ctx, sessionID, state)
	return err
}

func roleplaySceneDraftStateKey(channelID model.ChannelID) string {
	return "roleplay_scene_draft:" + string(channelID)
}

func (s *Server) validateRoleplaySceneDraftSelectionLocked(
	ctx context.Context,
	sessionID string,
	channelID model.ChannelID,
	worldID string,
	expectedRevision int64,
	expectedSceneRevision int64,
	participantIDs []string,
) error {
	draft, err := s.loadRoleplaySceneDraftLocked(ctx, sessionID, channelID, worldID)
	if err != nil {
		return err
	}
	if draft.Revision != expectedRevision {
		return roleplay.ErrSimulationStaleRevision
	}
	if draft.SceneRevision != expectedSceneRevision {
		return roleplay.ErrSimulationStaleRevision
	}
	if len(draft.Participants) != len(participantIDs) {
		return fmt.Errorf("%w: scene participants differ from the server draft", roleplay.ErrSimulationConflict)
	}
	for index, participant := range draft.Participants {
		if participant.CharacterID != participantIDs[index] {
			return fmt.Errorf("%w: scene participant order differs from the server draft", roleplay.ErrSimulationConflict)
		}
	}
	return nil
}

func (s *Server) reconcileRoleplaySceneDraftLocked(
	ctx context.Context,
	sessionID string,
	draft roleplaySceneDraft,
	state roleplaySimulationComponentState,
) (roleplaySceneDraft, error) {
	expectedSceneRevision := int64(0)
	participants := []roleplaySceneDraftParticipant{}
	if state.Scene != nil {
		expectedSceneRevision = state.Scene.Revision
		participants = make([]roleplaySceneDraftParticipant, len(state.AllParticipants))
		for index, participant := range state.AllParticipants {
			participants[index] = roleplaySceneDraftParticipant{CharacterID: participant.CharacterID}
		}
	}
	if draft.SceneRevision == expectedSceneRevision {
		return draft, nil
	}
	draft.SceneRevision = expectedSceneRevision
	draft.Participants = participants
	draft.Revision++
	if err := s.persistRoleplaySceneDraftLocked(ctx, sessionID, draft); err != nil {
		return roleplaySceneDraft{}, err
	}
	return draft, nil
}

func validateRoleplaySceneDraft(
	draft roleplaySceneDraft,
	channelID model.ChannelID,
	worldID string,
) error {
	if draft.Schema != roleplaySceneDraftSchema || draft.ChannelID != channelID || draft.WorldID != worldID ||
		draft.Revision < 0 || draft.SceneRevision < 0 {
		return fmt.Errorf("persisted roleplay scene draft changed authority")
	}
	if len(draft.Participants) > roleplay.MaxSceneParticipants {
		return fmt.Errorf("persisted roleplay scene draft exceeds participant bound")
	}
	seen := make(map[string]struct{}, len(draft.Participants))
	for _, participant := range draft.Participants {
		if !roleplayCharacterIdentityPattern.MatchString(participant.CharacterID) {
			return fmt.Errorf("persisted roleplay scene draft participant is invalid")
		}
		if _, exists := seen[participant.CharacterID]; exists {
			return fmt.Errorf("persisted roleplay scene draft participant is duplicated")
		}
		seen[participant.CharacterID] = struct{}{}
	}
	return nil
}
