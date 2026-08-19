package roleplay

import (
	"encoding/json"
	"fmt"
	"slices"
)

func (authority SimulationTurnAuthority) Validate() error {
	if validateIdentity(authority.PreparationID, transitionIdentity) != nil ||
		validateChannelID(authority.ChannelID) != nil || authority.UserMessageID < 1 ||
		validateIdentity(authority.WorldID, worldIdentity) != nil ||
		validateIdentity(authority.SceneID, sceneIdentity) != nil ||
		validateIdentity(authority.ActiveCharacterID, characterIdentity) != nil ||
		authority.BaseSceneRevision < 1 || authority.SceneRevision < authority.BaseSceneRevision ||
		authority.SceneRevision > authority.BaseSceneRevision+1 ||
		!validSimulationSHA(authority.NarrativeFingerprint) ||
		authority.CreatedAt.IsZero() {
		return fmt.Errorf("simulation turn authority contains invalid identity, revision, fingerprint, or time")
	}
	if authority.InputKind != SimulationTurnProse && authority.InputKind != SimulationTurnAction &&
		authority.InputKind != SimulationTurnExternalCommand {
		return fmt.Errorf("simulation turn authority input kind is invalid")
	}
	if len(authority.ParticipantCharacterIDs) < 1 || len(authority.ParticipantCharacterIDs) > MaxSceneParticipants {
		return fmt.Errorf("simulation turn authority participant count is outside its bound")
	}
	seen := make(map[string]struct{}, len(authority.ParticipantCharacterIDs))
	activeFound := false
	for _, id := range authority.ParticipantCharacterIDs {
		if validateIdentity(id, characterIdentity) != nil {
			return fmt.Errorf("simulation turn authority participant is invalid")
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("simulation turn authority participant is duplicated")
		}
		seen[id] = struct{}{}
		activeFound = activeFound || id == authority.ActiveCharacterID
	}
	if !activeFound {
		return fmt.Errorf("simulation turn authority active character is not a participant")
	}
	if authority.ExplicitAction != (authority.InputKind == SimulationTurnAction) {
		return fmt.Errorf("simulation turn authority input kind does not match action authority")
	}
	if authority.ExplicitAction && authority.PendingTransition == nil {
		return fmt.Errorf("simulation turn action lost its pending transition")
	}
	if authority.PendingTransition == nil {
		if authority.SceneRevision != authority.BaseSceneRevision {
			return fmt.Errorf("simulation turn without a pending transition changed revision")
		}
	} else {
		transition := authority.PendingTransition
		if transition.OperationID != authority.PreparationID ||
			transition.WorldID != authority.WorldID || transition.SceneID != authority.SceneID ||
			transition.ActorCharacterID != authority.ActiveCharacterID ||
			transition.BeforeRevision != authority.BaseSceneRevision ||
			transition.AfterRevision != authority.SceneRevision {
			return fmt.Errorf("simulation pending transition differs from turn authority")
		}
		payload, err := json.Marshal(transition)
		if err != nil {
			return fmt.Errorf("encode simulation pending transition: %w", err)
		}
		if _, err := decodeSimulationTransitionResult(payload); err != nil {
			return fmt.Errorf("simulation pending transition is invalid: %w", err)
		}
	}
	if err := authority.NarrativeProjection.Validate(); err != nil {
		return fmt.Errorf("simulation turn narrative projection: %w", err)
	}
	narrative := authority.NarrativeAuthority
	if narrative.WorldID != authority.WorldID || narrative.SceneID != authority.SceneID ||
		narrative.SceneRevision != authority.SceneRevision ||
		narrative.ViewpointID != authority.ActiveCharacterID ||
		!slices.Equal(narrative.ParticipantIDs, authority.ParticipantCharacterIDs) ||
		narrative.Fingerprint != authority.NarrativeFingerprint {
		return fmt.Errorf("simulation narrative authority differs from turn authority")
	}
	digest, err := simulationNarrativeDigest(authority.NarrativeProjection, narrative)
	if err != nil || digest != authority.NarrativeFingerprint {
		return fmt.Errorf("simulation narrative projection differs from its fingerprint")
	}
	return nil
}
