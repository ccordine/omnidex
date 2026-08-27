package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/roleplay"
)

func filterNewRoleplayCanonFacts(
	ctx context.Context,
	filter func(context.Context, string, []string) ([]string, error),
	worldID string,
	candidates []string,
) ([]string, error) {
	if len(candidates) == 0 {
		return []string{}, nil
	}
	if filter == nil {
		return nil, fmt.Errorf("roleplay world-canon delta authority is unavailable")
	}
	filtered, err := filter(ctx, worldID, append([]string{}, candidates...))
	if err != nil {
		return nil, err
	}
	position := 0
	seen := make(map[string]struct{}, len(filtered))
	for _, fact := range filtered {
		if _, duplicate := seen[fact]; duplicate {
			return nil, fmt.Errorf("roleplay world-canon delta duplicated a fact")
		}
		seen[fact] = struct{}{}
		for position < len(candidates) && candidates[position] != fact {
			position++
		}
		if position == len(candidates) {
			return nil, fmt.Errorf("roleplay world-canon delta introduced or reordered a fact")
		}
		position++
	}
	return append([]string{}, filtered...), nil
}

func extractRoleplayCanonSource(
	ctx context.Context,
	station objectiveRoleplayCanonStation,
	input assemblyline.RoleplayCanonExtractionInput,
) ([]string, int, error) {
	if _, err := assemblyline.NewRoleplayCanonExtractionJob(input); err != nil {
		return nil, 0, err
	}
	decision, receipt, err := station.ExtractCanon(ctx, input)
	if err != nil {
		return nil, 0, err
	}
	if receipt.Calls < 1 || receipt.Calls > maxTypedWorkerAttempts {
		return nil, 0, fmt.Errorf(
			"roleplay canon extraction reported %d calls outside the bounded correction budget",
			receipt.Calls,
		)
	}
	if err := decision.ValidateFor(input); err != nil {
		return nil, 0, err
	}
	return append([]string{}, decision.Facts...), receipt.Calls, nil
}

func roleplayUserContributionRequiresCanon(authority roleplay.UserTurnAuthority) bool {
	return authority.ContributionKind != roleplay.UserContributionCommand &&
		authority.ContributionKind != roleplay.UserContributionLegacy
}

func newRoleplayUserCanonCompletion(
	preparation roleplay.SimulationTurnAuthority,
	facts []string,
) (*queue.RoleplayUserCanonCompletion, error) {
	if !roleplayUserContributionRequiresCanon(preparation.UserTurn) {
		if len(facts) != 0 {
			return nil, fmt.Errorf("roleplay command cannot persist user-contribution canon")
		}
		return nil, nil
	}
	completion := &queue.RoleplayUserCanonCompletion{Facts: append([]string{}, facts...)}
	if len(facts) == 0 {
		completion.KnowledgeCharacterIDs = []model.RoleplayCharacterID{}
		return completion, nil
	}
	switch preparation.UserTurn.PersonaKind {
	case roleplay.UserPersonaCharacter:
		completion.KnowledgeCharacterIDs = []model.RoleplayCharacterID{
			model.RoleplayCharacterID(preparation.UserTurn.CharacterID),
		}
	case roleplay.UserPersonaNarrator:
		completion.KnowledgeCharacterIDs = make(
			[]model.RoleplayCharacterID, len(preparation.ParticipantCharacterIDs),
		)
		for index, characterID := range preparation.ParticipantCharacterIDs {
			completion.KnowledgeCharacterIDs[index] = model.RoleplayCharacterID(characterID)
		}
	default:
		return nil, fmt.Errorf(
			"roleplay user canon has unsupported persona kind %q",
			preparation.UserTurn.PersonaKind,
		)
	}
	return completion, nil
}
