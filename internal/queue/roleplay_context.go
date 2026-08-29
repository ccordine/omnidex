package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func (r *Repository) ProjectRoleplayCharacterContext(
	ctx context.Context,
	channelID model.ChannelID,
	viewpointID model.RoleplayCharacterID,
	limit int,
) (roleplay.CharacterProjection, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return roleplay.CharacterProjection{}, fmt.Errorf("roleplay context projection requires PostgreSQL and context")
	}
	if err := viewpointID.Validate(); err != nil {
		return roleplay.CharacterProjection{}, err
	}
	if err := channelID.Validate(); err != nil {
		return roleplay.CharacterProjection{}, err
	}
	store, err := roleplay.NewStore(r.pool)
	if err != nil {
		return roleplay.CharacterProjection{}, err
	}
	projection, err := store.ProjectChannelCharacterContext(
		ctx, string(channelID), string(viewpointID), limit,
	)
	if err != nil {
		return roleplay.CharacterProjection{}, err
	}
	if err := projection.Validate(); err != nil {
		return roleplay.CharacterProjection{}, fmt.Errorf("invalid persisted roleplay context: %w", err)
	}
	return projection, nil
}

func (r *Repository) ProjectRoleplaySimulationContext(
	ctx context.Context,
	preparationID string,
	jobID int64,
) (roleplay.SimulationTurnAuthority, roleplay.NarrativeSimulationProjection, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return roleplay.SimulationTurnAuthority{}, roleplay.NarrativeSimulationProjection{},
			fmt.Errorf("roleplay simulation projection requires PostgreSQL and context")
	}
	store, err := roleplay.NewStore(r.pool)
	if err != nil {
		return roleplay.SimulationTurnAuthority{}, roleplay.NarrativeSimulationProjection{}, err
	}
	preparation, err := store.LoadFreshSimulationTurnForJob(ctx, preparationID, jobID)
	if err != nil {
		return roleplay.SimulationTurnAuthority{}, roleplay.NarrativeSimulationProjection{}, err
	}
	if len(preparation.Responders) < 1 {
		return roleplay.SimulationTurnAuthority{}, roleplay.NarrativeSimulationProjection{},
			fmt.Errorf("prepared roleplay turn has no response round")
	}
	primary := preparation.Responders[0]
	projection := roleplay.CloneNarrativeSimulationProjection(primary.NarrativeProjection)
	authority := preparation.NarrativeAuthority
	if err := projection.Validate(); err != nil {
		return roleplay.SimulationTurnAuthority{}, roleplay.NarrativeSimulationProjection{},
			fmt.Errorf("invalid persisted roleplay simulation narrative: %w", err)
	}
	if authority.WorldID != preparation.WorldID || authority.SceneID != preparation.SceneID ||
		authority.SceneRevision != preparation.SceneRevision ||
		authority.ViewpointID != primary.CharacterID ||
		authority.Fingerprint != preparation.NarrativeFingerprint ||
		len(authority.ParticipantIDs) != len(preparation.ParticipantCharacterIDs) {
		return roleplay.SimulationTurnAuthority{}, roleplay.NarrativeSimulationProjection{},
			fmt.Errorf("roleplay narrative projection differs from prepared turn authority")
	}
	for index, participantID := range authority.ParticipantIDs {
		if participantID != preparation.ParticipantCharacterIDs[index] {
			return roleplay.SimulationTurnAuthority{}, roleplay.NarrativeSimulationProjection{},
				fmt.Errorf("roleplay narrative participants differ from prepared turn authority")
		}
	}
	return preparation, roleplay.CloneNarrativeSimulationProjection(projection), nil
}
