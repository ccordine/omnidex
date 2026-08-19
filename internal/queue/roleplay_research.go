package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/roleplay"
)

func (r *Repository) ConfigureRoleplayCharacterCapability(
	ctx context.Context,
	worldID, characterID string,
	capability roleplay.CharacterCapability,
	enabled bool,
) (roleplay.CharacterCapabilityProjection, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return roleplay.CharacterCapabilityProjection{}, fmt.Errorf("roleplay capability configuration requires PostgreSQL and context")
	}
	store, err := roleplay.NewStore(r.pool)
	if err != nil {
		return roleplay.CharacterCapabilityProjection{}, err
	}
	return store.ConfigureCharacterCapability(ctx, worldID, characterID, capability, enabled)
}

func (r *Repository) ProjectRoleplayCharacterCapability(
	ctx context.Context,
	worldID, characterID string,
) (roleplay.CharacterCapabilityProjection, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return roleplay.CharacterCapabilityProjection{}, fmt.Errorf("roleplay capability projection requires PostgreSQL and context")
	}
	store, err := roleplay.NewStore(r.pool)
	if err != nil {
		return roleplay.CharacterCapabilityProjection{}, err
	}
	return store.ProjectCharacterCapability(ctx, worldID, characterID)
}

func (r *Repository) LoadRoleplayResearchTurn(
	ctx context.Context,
	jobID int64,
) (roleplay.ResearchTurnAuthority, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return roleplay.ResearchTurnAuthority{}, fmt.Errorf("roleplay research lookup requires PostgreSQL and context")
	}
	store, err := roleplay.NewStore(r.pool)
	if err != nil {
		return roleplay.ResearchTurnAuthority{}, err
	}
	return store.LoadResearchTurnForJob(ctx, jobID)
}

func (r *Repository) ProjectRoleplayResearchNarrative(
	ctx context.Context,
	research roleplay.ResearchTurnAuthority,
	jobID int64,
) (roleplay.NarrativeSimulationProjection, roleplay.SimulationNarrativeAuthority, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return roleplay.NarrativeSimulationProjection{}, roleplay.SimulationNarrativeAuthority{},
			fmt.Errorf("roleplay research narrative projection requires PostgreSQL and context")
	}
	if err := research.Validate(); err != nil {
		return roleplay.NarrativeSimulationProjection{}, roleplay.SimulationNarrativeAuthority{}, err
	}
	store, err := roleplay.NewStore(r.pool)
	if err != nil {
		return roleplay.NarrativeSimulationProjection{}, roleplay.SimulationNarrativeAuthority{}, err
	}
	preparation, err := store.LoadSimulationTurnForJob(ctx, research.PreparationID, jobID)
	if err != nil {
		return roleplay.NarrativeSimulationProjection{}, roleplay.SimulationNarrativeAuthority{}, err
	}
	projection := roleplay.CloneNarrativeSimulationProjection(preparation.NarrativeProjection)
	authority := preparation.NarrativeAuthority
	if err := roleplay.ValidateResearchNarrativeProjection(projection); err != nil {
		return roleplay.NarrativeSimulationProjection{}, roleplay.SimulationNarrativeAuthority{}, err
	}
	if authority.WorldID != research.WorldID || authority.SceneID != research.SceneID ||
		authority.SceneRevision != research.SceneRevision || authority.ViewpointID != research.CharacterID ||
		authority.Fingerprint != research.NarrativeFingerprint {
		return roleplay.NarrativeSimulationProjection{}, roleplay.SimulationNarrativeAuthority{},
			fmt.Errorf("roleplay research narrative differs from prepared active-character authority")
	}
	return roleplay.CloneResearchNarrativeProjection(projection), authority, nil
}
