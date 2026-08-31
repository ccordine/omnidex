package worker

import (
	"context"
	"fmt"
	"slices"

	"github.com/gryph/omnidex/internal/contextcompiler"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/roleplay"
)

func (provider boundObjectiveContextProvider) SearchAvailability(
	ctx context.Context,
) (contextcompiler.SearchAvailability, error) {
	if ctx == nil {
		return "", fmt.Errorf("context search availability requires a context")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := provider.validateAuthority(); err != nil {
		return "", err
	}
	switch provider.authority.ChannelMode {
	case model.ChannelModeAssistant:
		return provider.assistantSearchAvailability(ctx)
	case model.ChannelModeRoleplay:
		return provider.roleplaySearchAvailability(ctx)
	default:
		return "", fmt.Errorf(
			"context search availability has unsupported channel mode %q",
			provider.authority.ChannelMode,
		)
	}
}

func (provider boundObjectiveContextProvider) assistantSearchAvailability(
	ctx context.Context,
) (contextcompiler.SearchAvailability, error) {
	recent, err := provider.runtime.svc.repo.ConversationCandidateAuthorities(ctx, provider.job)
	if err != nil {
		return "", err
	}
	recentRecords, err := recentConversationContextRecords(recent.Turns)
	if err != nil {
		return "", err
	}
	representedMessageIDs := make([]int64, len(recent.AssistantResults))
	for index, result := range recent.AssistantResults {
		representedMessageIDs[index] = result.UserMessageID
	}
	if len(recentRecords) != len(representedMessageIDs) {
		return "", fmt.Errorf("recent assistant context differs from exact result authority")
	}
	additionalConversation, err := provider.runtime.svc.repo.HasAdditionalConversationSearchAuthority(
		ctx, provider.job, representedMessageIDs,
	)
	if err != nil {
		return "", err
	}
	if additionalConversation {
		return contextcompiler.SearchAvailable, nil
	}
	return contextcompiler.SearchUnavailable, nil
}

func (provider boundObjectiveContextProvider) roleplaySearchAvailability(
	ctx context.Context,
) (contextcompiler.SearchAvailability, error) {
	preparation, _, responder, err := provider.roleplayContextAuthority()
	if err != nil {
		return "", err
	}
	conversation, err := provider.runtime.svc.repo.RoleplayConversationCandidateAuthorities(
		ctx, provider.job, model.RoleplayCharacterID(responder.CharacterID),
	)
	if err != nil {
		return "", err
	}
	completedTurns, err := completedConversationCandidateTurns(conversation.Turns)
	if err != nil {
		return "", err
	}
	conversationRecords, err := recentConversationContextRecords(completedTurns)
	if err != nil {
		return "", err
	}
	conversationSourceIDs := make([]string, len(conversationRecords))
	for index, record := range conversationRecords {
		conversationSourceIDs[index] = record.SourceID
	}
	persistedEvents, err := representedPersistedSimulationEvents(
		preparation, responder,
	)
	if err != nil {
		return "", err
	}
	additional, err := provider.runtime.svc.repo.HasAdditionalRoleplaySearchAuthority(
		ctx, preparation.WorldID, model.RoleplayCharacterID(responder.CharacterID),
		preparation.SceneID, preparation.CreatedAt,
		queue.RoleplayContextSearchRepresentation{
			CanonEventIDs:           responder.NarrativeAuthority.CanonEventIDs,
			MemoryIDs:               responder.NarrativeAuthority.MemoryIDs,
			ConversationSourceIDs:   conversationSourceIDs,
			SimulationEventContents: persistedEvents,
		},
	)
	if err != nil {
		return "", err
	}
	if additional {
		return contextcompiler.SearchAvailable, nil
	}
	return contextcompiler.SearchUnavailable, nil
}

func representedPersistedSimulationEvents(
	preparation roleplay.SimulationTurnAuthority,
	responder roleplay.SimulationResponderAuthority,
) ([]string, error) {
	events := responder.NarrativeProjection.RecentEvents
	if preparation.PendingTransition == nil {
		return append([]string{}, events...), nil
	}
	pending := preparation.PendingTransition
	if len(events) < len(pending.NarrativeEvents) ||
		!slices.Equal(events[len(events)-len(pending.NarrativeEvents):], pending.NarrativeEvents) ||
		len(responder.NarrativeAuthority.TransitionIDs) == 0 ||
		responder.NarrativeAuthority.TransitionIDs[len(responder.NarrativeAuthority.TransitionIDs)-1] != pending.OperationID {
		return nil, fmt.Errorf(
			"frozen roleplay projection does not identify its pending transition suffix",
		)
	}
	return append([]string{}, events[:len(events)-len(pending.NarrativeEvents)]...), nil
}
