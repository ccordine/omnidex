package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/jackc/pgx/v5"
)

type roleplayPortableReuseResponderAuthority struct {
	Position             int    `json:"position"`
	CharacterID          string `json:"character_id"`
	NarrativeFingerprint string `json:"narrative_fingerprint"`
}

// roleplayPortableReuseAuthority deliberately excludes preparation/message
// identities and generation configuration. Those change on an exact retry and
// do not alter the listed fictional question or its accepted semantic answer.
type roleplayPortableReuseAuthority struct {
	ChannelID               string                                    `json:"channel_id"`
	WorldID                 string                                    `json:"world_id"`
	SceneID                 string                                    `json:"scene_id"`
	SceneRevision           int64                                     `json:"scene_revision"`
	InputKind               roleplay.SimulationTurnInputKind          `json:"input_kind"`
	UserTurn                json.RawMessage                           `json:"user_turn"`
	ParticipantCharacterIDs []model.RoleplayCharacterID               `json:"participant_character_ids"`
	Responders              []roleplayPortableReuseResponderAuthority `json:"responders"`
}

func canonicalRoleplayPortableReuseAuthority(job model.Job) ([]byte, error) {
	if job.Pipeline != model.PipelineChat {
		return nil, fmt.Errorf("roleplay portable reuse job %d is not a chat job", job.ID)
	}
	var metadata channelTurnMetadata
	if err := json.Unmarshal(job.Metadata, &metadata); err != nil {
		return nil, fmt.Errorf("decode roleplay portable reuse job %d metadata: %w", job.ID, err)
	}
	if err := validateChannelTurnMetadata(metadata); err != nil {
		return nil, fmt.Errorf("roleplay portable reuse job %d metadata: %w", job.ID, err)
	}
	if metadata.ChannelMode != model.ChannelModeRoleplay || metadata.RoleplayUserTurn == nil {
		return nil, fmt.Errorf("roleplay portable reuse job %d has no fictional authority", job.ID)
	}
	if job.Instruction != metadata.RoleplayUserTurn.ExactText {
		return nil, fmt.Errorf(
			"roleplay portable reuse job %d instruction differs from its exact user turn",
			job.ID,
		)
	}

	var rawMetadata map[string]json.RawMessage
	if err := json.Unmarshal(job.Metadata, &rawMetadata); err != nil {
		return nil, fmt.Errorf("decode roleplay portable reuse raw metadata: %w", err)
	}
	rawUserTurn, exists := rawMetadata["roleplay_user_turn"]
	if !exists || len(rawUserTurn) == 0 || bytes.Equal(rawUserTurn, []byte("null")) {
		return nil, fmt.Errorf("roleplay portable reuse job %d has no exact user-turn JSON", job.ID)
	}
	canonicalUserTurn, err := exactjson.Canonical(json.RawMessage(rawUserTurn))
	if err != nil {
		return nil, fmt.Errorf("canonicalize roleplay portable reuse user turn: %w", err)
	}

	responders := make([]roleplayPortableReuseResponderAuthority, len(metadata.RoleplayResponders))
	for index, responder := range metadata.RoleplayResponders {
		responders[index] = roleplayPortableReuseResponderAuthority{
			Position: responder.Position, CharacterID: responder.CharacterID,
			NarrativeFingerprint: responder.NarrativeFingerprint,
		}
	}
	authority := roleplayPortableReuseAuthority{
		ChannelID: string(metadata.ChannelID), WorldID: metadata.RoleplayWorldID,
		SceneID: metadata.RoleplaySceneID, SceneRevision: metadata.RoleplaySceneRevision,
		InputKind: metadata.RoleplayInputKind, UserTurn: canonicalUserTurn,
		ParticipantCharacterIDs: append(
			[]model.RoleplayCharacterID(nil), metadata.RoleplayParticipantCharacterIDs...,
		),
		Responders: responders,
	}
	canonical, err := exactjson.Canonical(authority)
	if err != nil {
		return nil, fmt.Errorf("canonicalize roleplay portable reuse authority: %w", err)
	}
	return canonical, nil
}

func requireRoleplayPortableReuseJobBindingTx(
	ctx context.Context,
	tx pgx.Tx,
	job model.Job,
) error {
	var metadata channelTurnMetadata
	if err := json.Unmarshal(job.Metadata, &metadata); err != nil {
		return fmt.Errorf("decode roleplay portable reuse binding metadata: %w", err)
	}
	var raw []byte
	err := tx.QueryRow(ctx, `
		SELECT preparation.result
		FROM roleplay_simulation_turn_preparations AS preparation
		JOIN roleplay_simulation_preparation_jobs AS binding
		  ON binding.preparation_id=preparation.operation_id
		WHERE preparation.operation_id=$1 AND binding.job_id=$2
		FOR SHARE OF preparation,binding
	`, metadata.RoleplaySimulationPreparationID, job.ID).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf(
			"roleplay portable reuse job %d has no exact simulation preparation binding",
			job.ID,
		)
	}
	if err != nil {
		return err
	}
	var preparation roleplay.SimulationTurnAuthority
	if err := json.Unmarshal(raw, &preparation); err != nil {
		return fmt.Errorf("decode roleplay portable reuse simulation preparation: %w", err)
	}
	if err := preparation.Validate(); err != nil {
		return fmt.Errorf("roleplay portable reuse simulation preparation: %w", err)
	}
	participants := make([]string, len(metadata.RoleplayParticipantCharacterIDs))
	for index, participant := range metadata.RoleplayParticipantCharacterIDs {
		participants[index] = string(participant)
	}
	if preparation.PreparationID != metadata.RoleplaySimulationPreparationID ||
		preparation.ChannelID != string(metadata.ChannelID) ||
		preparation.UserMessageID != metadata.ChannelUserMessageID ||
		preparation.WorldID != metadata.RoleplayWorldID ||
		preparation.SceneID != metadata.RoleplaySceneID ||
		preparation.SceneRevision != metadata.RoleplaySceneRevision ||
		preparation.InputKind != metadata.RoleplayInputKind ||
		preparation.NarrativeFingerprint != metadata.RoleplayNarrativeFingerprint ||
		!slices.Equal(preparation.ParticipantCharacterIDs, participants) ||
		!slices.Equal(preparation.ResponderRoutes, metadata.RoleplayResponders) ||
		metadata.RoleplayUserTurn == nil || !preparation.UserTurn.Equal(*metadata.RoleplayUserTurn) {
		return fmt.Errorf(
			"roleplay portable reuse job %d differs from its simulation preparation",
			job.ID,
		)
	}
	return nil
}
