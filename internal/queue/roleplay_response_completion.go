package queue

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func RenderRoleplayResponseRound(responses []RoleplayResponseCompletion) string {
	outputs := make([]string, len(responses))
	for index, response := range responses {
		outputs[index] = response.Output
	}
	return strings.Join(outputs, "\n\n")
}

// ValidateRoleplayResponseRound applies the authoritative per-response and
// ordered-round bounds without treating the synthetic joined projection as a
// single persisted channel message.
func ValidateRoleplayResponseRound(responses []RoleplayResponseCompletion) error {
	_, err := normalizeRoleplayResponseCompletions(responses)
	return err
}

func normalizeRoleplayResponseCompletions(
	responses []RoleplayResponseCompletion,
) ([]RoleplayResponseCompletion, error) {
	if len(responses) == 0 {
		return nil, nil
	}
	if len(responses) > roleplay.MaxSceneParticipants {
		return nil, fmt.Errorf("roleplay completion response round exceeds its participant bound")
	}
	normalized := make([]RoleplayResponseCompletion, len(responses))
	seenCharacters := make(map[model.RoleplayCharacterID]struct{}, len(responses))
	seenFacts := make(map[string]struct{})
	for index, response := range responses {
		if response.Position != index {
			return nil, fmt.Errorf("roleplay completion response %d has invalid order", index)
		}
		if err := response.CharacterID.Validate(); err != nil {
			return nil, fmt.Errorf("roleplay completion response %d character: %w", index, err)
		}
		if _, duplicate := seenCharacters[response.CharacterID]; duplicate {
			return nil, fmt.Errorf("roleplay completion character %q is duplicated", response.CharacterID)
		}
		seenCharacters[response.CharacterID] = struct{}{}
		if err := model.ValidateChannelMessage(model.ChannelMessageRoleAssistant, response.Output); err != nil {
			return nil, fmt.Errorf("roleplay completion response %d output: %w", index, err)
		}
		if len(response.Output) > roleplay.MaxNarrativeResponseBytes {
			return nil, fmt.Errorf(
				"roleplay completion response %d output exceeds the %d-byte narrative bound",
				index, roleplay.MaxNarrativeResponseBytes,
			)
		}
		if len(response.Facts) > roleplay.MaxCanonFactsPerTurn {
			return nil, fmt.Errorf("roleplay completion response %d exceeds its fact bound", index)
		}
		facts := append([]string{}, response.Facts...)
		for factIndex, fact := range facts {
			if err := roleplay.ValidateCanonFact(fact); err != nil {
				return nil, fmt.Errorf("roleplay completion response %d fact %d: %w", index, factIndex, err)
			}
			if _, duplicate := seenFacts[fact]; duplicate {
				return nil, fmt.Errorf("roleplay completion fact %q is duplicated in the response round", fact)
			}
			seenFacts[fact] = struct{}{}
		}
		knowledge := append([]model.RoleplayCharacterID{}, response.KnowledgeCharacterIDs...)
		if len(facts) == 0 && len(knowledge) != 0 {
			return nil, fmt.Errorf("roleplay response knowledge requires new canon facts")
		}
		if len(facts) != 0 && (len(knowledge) != 1 || knowledge[0] != response.CharacterID) {
			return nil, fmt.Errorf("roleplay response knowledge must bind to its responding character")
		}
		var previousOngoingAction *string
		if response.PreviousOngoingAction != nil {
			if err := roleplay.ValidateOngoingActionText(*response.PreviousOngoingAction); err != nil {
				return nil, fmt.Errorf("roleplay completion response %d previous ongoing action: %w", index, err)
			}
			copy := *response.PreviousOngoingAction
			previousOngoingAction = &copy
		}
		var ongoingAction *string
		if response.OngoingAction != nil {
			if err := roleplay.ValidateOngoingActionText(*response.OngoingAction); err != nil {
				return nil, fmt.Errorf("roleplay completion response %d ongoing action: %w", index, err)
			}
			copy := *response.OngoingAction
			ongoingAction = &copy
		}
		normalized[index] = RoleplayResponseCompletion{
			Position: index, CharacterID: response.CharacterID, Output: response.Output,
			Facts: facts, KnowledgeCharacterIDs: knowledge,
			PreviousOngoingAction: previousOngoingAction, OngoingAction: ongoingAction,
		}
	}
	return normalized, nil
}

func normalizeRoleplayUserCanonCompletion(
	completion *RoleplayUserCanonCompletion,
) (*RoleplayUserCanonCompletion, error) {
	if completion == nil {
		return nil, nil
	}
	if len(completion.Facts) > roleplay.MaxCanonFactsPerTurn {
		return nil, fmt.Errorf("roleplay user canon exceeds its fact bound")
	}
	facts := append([]string{}, completion.Facts...)
	seenFacts := make(map[string]struct{}, len(facts))
	for index, fact := range facts {
		if err := roleplay.ValidateCanonFact(fact); err != nil {
			return nil, fmt.Errorf("roleplay user canon fact %d: %w", index, err)
		}
		if _, duplicate := seenFacts[fact]; duplicate {
			return nil, fmt.Errorf("roleplay user canon fact %q is duplicated", fact)
		}
		seenFacts[fact] = struct{}{}
	}
	if len(completion.KnowledgeCharacterIDs) > roleplay.MaxKnowledgeRecipientsPerTurn {
		return nil, fmt.Errorf("roleplay user canon exceeds its knowledge-recipient bound")
	}
	knowledge := append([]model.RoleplayCharacterID{}, completion.KnowledgeCharacterIDs...)
	if len(facts) == 0 && len(knowledge) != 0 {
		return nil, fmt.Errorf("roleplay user canon knowledge requires new facts")
	}
	if len(facts) != 0 && len(knowledge) == 0 {
		return nil, fmt.Errorf("roleplay user canon facts require exact knowledge recipients")
	}
	seenCharacters := make(map[model.RoleplayCharacterID]struct{}, len(knowledge))
	for index, characterID := range knowledge {
		if err := characterID.Validate(); err != nil {
			return nil, fmt.Errorf("roleplay user canon recipient %d: %w", index, err)
		}
		if _, duplicate := seenCharacters[characterID]; duplicate {
			return nil, fmt.Errorf("roleplay user canon recipient %q is duplicated", characterID)
		}
		seenCharacters[characterID] = struct{}{}
	}
	return &RoleplayUserCanonCompletion{
		Facts: facts, KnowledgeCharacterIDs: knowledge,
	}, nil
}

func normalizeRoleplayUserOngoingActionCompletion(
	completion *RoleplayUserOngoingActionCompletion,
) (*RoleplayUserOngoingActionCompletion, error) {
	if completion == nil {
		return nil, nil
	}
	if err := completion.CharacterID.Validate(); err != nil {
		return nil, fmt.Errorf("roleplay user ongoing-action character: %w", err)
	}
	var previous *string
	if completion.PreviousOngoingAction != nil {
		if err := roleplay.ValidateOngoingActionText(*completion.PreviousOngoingAction); err != nil {
			return nil, fmt.Errorf("roleplay user previous ongoing action: %w", err)
		}
		copy := *completion.PreviousOngoingAction
		previous = &copy
	}
	var current *string
	if completion.OngoingAction != nil {
		if err := roleplay.ValidateOngoingActionText(*completion.OngoingAction); err != nil {
			return nil, fmt.Errorf("roleplay user ongoing action: %w", err)
		}
		copy := *completion.OngoingAction
		current = &copy
	}
	return &RoleplayUserOngoingActionCompletion{
		CharacterID:           completion.CharacterID,
		PreviousOngoingAction: previous, OngoingAction: current,
	}, nil
}
