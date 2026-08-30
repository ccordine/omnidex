package worker

import (
	"context"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/contextcompiler"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

type scriptedConversationCandidateProvider struct {
	contextSet        contextcompiler.CandidateSet
	err               error
	availability      contextcompiler.SearchAvailability
	availabilityError error
}

func (provider scriptedConversationCandidateProvider) ContextSearchAvailability(
	context.Context,
	model.Job,
	turnAuthority,
	*roleplay.SimulationTurnAuthority,
	*roleplay.NarrativeSimulationProjection,
) (contextcompiler.SearchAvailability, error) {
	if provider.availability == "" {
		return contextcompiler.SearchAvailable, provider.availabilityError
	}
	return provider.availability, provider.availabilityError
}

func (provider scriptedConversationCandidateProvider) ContextCandidates(
	context.Context,
	model.Job,
	turnAuthority,
	*roleplay.SimulationTurnAuthority,
	*roleplay.NarrativeSimulationProjection,
	[]string,
) (contextcompiler.CandidateSet, error) {
	return provider.contextSet, provider.err
}

type scriptedConversationContextStation struct {
	relevantIDs        []string
	relevantIDsByCall  [][]string
	minimalContext     string
	relevanceCalls     int
	relevanceInputs    []assemblyline.ContextRelevanceRelationInput
	minificationCalls  int
	minificationInputs []assemblyline.ContextMinificationInput
}

func (station *scriptedConversationContextStation) Relate(
	_ context.Context,
	input assemblyline.ContextRelevanceRelationInput,
) (assemblyline.ContextRelevanceRelationResult, contextcompiler.StationReceipt, error) {
	callIndex := station.relevanceCalls
	station.relevanceCalls++
	station.relevanceInputs = append(station.relevanceInputs, input)
	raw := assemblyline.ContextCandidateNotDirectlyRelevant
	relevantIDs := station.relevantIDs
	if callIndex < len(station.relevantIDsByCall) {
		relevantIDs = station.relevantIDsByCall[callIndex]
	}
	for _, candidateID := range relevantIDs {
		if input.Candidate.CandidateID == candidateID {
			raw = assemblyline.ContextCandidateDirectlyRelevant
			break
		}
	}
	decision, err := assemblyline.DecodeContextRelevanceRelationResult(input, raw)
	return decision, contextcompiler.StationReceipt{Calls: 1}, err
}

func (station *scriptedConversationContextStation) Minify(
	_ context.Context,
	input assemblyline.ContextMinificationInput,
) (assemblyline.ContextMinificationDecision, contextcompiler.StationReceipt, error) {
	station.minificationCalls++
	station.minificationInputs = append(station.minificationInputs, input)
	decision := assemblyline.ContextMinificationDecision{
		Schema:         assemblyline.ContextMinificationSchemaV1,
		MinimalContext: station.minimalContext,
	}
	return decision, contextcompiler.StationReceipt{Calls: 1}, decision.ValidateFor(input)
}

func emptyContextSieveStation() *scriptedConversationContextStation {
	return &scriptedConversationContextStation{relevantIDs: []string{}}
}

func answerObjectiveKindStation() *scriptedObjectiveKindStation {
	return &scriptedObjectiveKindStation{decision: assemblyline.ConversationObjectiveKindDecision{
		Schema: assemblyline.ConversationObjectiveKindSchemaV1,
		Kind:   assemblyline.ObjectiveKindAnswer,
	}}
}
