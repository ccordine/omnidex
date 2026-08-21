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
	if err := authority.UserTurn.Validate(); err != nil {
		return fmt.Errorf("simulation turn user authority: %w", err)
	}
	if authority.UserTurn.ExactText == "" {
		return fmt.Errorf("simulation turn requires exact user contribution bytes")
	}
	if (authority.InputKind == SimulationTurnProse && authority.UserTurn.ContributionKind == UserContributionCommand) ||
		(authority.InputKind != SimulationTurnProse && authority.UserTurn.ContributionKind != UserContributionCommand) {
		return fmt.Errorf("simulation turn input kind differs from user contribution authority")
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
	expectedResponderIDs := simulationResponderIDs(authority.ParticipantCharacterIDs, authority.UserTurn)
	if authority.UserTurn.PersonaKind == UserPersonaLegacy && len(authority.Responders) == 1 {
		expectedResponderIDs = []string{authority.Responders[0].CharacterID}
	}
	if len(expectedResponderIDs) < 1 || len(authority.Responders) != len(expectedResponderIDs) {
		return fmt.Errorf("simulation response round differs from its enabled participant authority")
	}
	if len(authority.ResponderRoutes) != len(authority.Responders) {
		return fmt.Errorf("simulation responder routes differ from the frozen response round")
	}
	for index, responder := range authority.Responders {
		if responder.Position != index || responder.CharacterID != expectedResponderIDs[index] {
			return fmt.Errorf("simulation responder order differs from enabled participant authority")
		}
		if err := responder.GenerationConfig.Validate(); err != nil {
			return fmt.Errorf("simulation responder %d generation config: %w", index, err)
		}
		route := authority.ResponderRoutes[index]
		if route.Position != responder.Position || route.CharacterID != responder.CharacterID ||
			route.GenerationConfig != responder.GenerationConfig ||
			route.NarrativeFingerprint != responder.NarrativeFingerprint {
			return fmt.Errorf("simulation responder route %d differs from its response authority", index)
		}
		if err := responder.NarrativeProjection.Validate(); err != nil {
			return fmt.Errorf("simulation responder %d narrative projection: %w", index, err)
		}
		narrative := responder.NarrativeAuthority
		if narrative.WorldID != authority.WorldID || narrative.SceneID != authority.SceneID ||
			narrative.SceneRevision != authority.SceneRevision || narrative.ViewpointID != responder.CharacterID ||
			!slices.Equal(narrative.ParticipantIDs, authority.ParticipantCharacterIDs) ||
			narrative.Fingerprint != responder.NarrativeFingerprint {
			return fmt.Errorf("simulation responder %d narrative authority differs from turn authority", index)
		}
		digest, err := simulationNarrativeDigest(responder.NarrativeProjection, narrative)
		if err != nil || digest != responder.NarrativeFingerprint {
			return fmt.Errorf("simulation responder %d narrative projection differs from its fingerprint", index)
		}
	}
	primary := authority.Responders[0]
	if authority.GenerationConfig != primary.GenerationConfig ||
		authority.NarrativeProjection.Schema != primary.NarrativeProjection.Schema ||
		authority.NarrativeFingerprint != primary.NarrativeFingerprint {
		return fmt.Errorf("simulation primary responder summary differs from its response round")
	}
	if err := requirePreparedNarrative(
		authority.NarrativeProjection, authority.NarrativeAuthority,
		primary.NarrativeProjection, primary.NarrativeAuthority,
	); err != nil {
		return fmt.Errorf("simulation primary responder summary: %w", err)
	}
	if err := authority.GenerationConfig.Validate(); err != nil {
		return fmt.Errorf("simulation turn character generation config: %w", err)
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
		narrative.ViewpointID != primary.CharacterID ||
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
