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
		normalized[index] = RoleplayResponseCompletion{
			Position: index, CharacterID: response.CharacterID, Output: response.Output,
			Facts: facts, KnowledgeCharacterIDs: knowledge,
		}
	}
	return normalized, nil
}
