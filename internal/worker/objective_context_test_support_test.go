package worker

import (
	"context"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/contextcompiler"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

type scriptedConversationCandidateProvider struct {
	contextSet contextcompiler.CandidateSet
	err        error
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
	terms             []string
	relevantIDs       []string
	minimalContext    string
	termCalls         int
	relevanceCalls    int
	minificationCalls int
}

func (station *scriptedConversationContextStation) Generate(
	_ context.Context,
	input assemblyline.ContextSearchTermsInput,
) (assemblyline.ContextSearchTermsDecision, contextcompiler.StationReceipt, error) {
	station.termCalls++
	decision := assemblyline.ContextSearchTermsDecision{
		Schema: assemblyline.ContextSearchTermsSchemaV1,
		Terms:  append([]string{}, station.terms...),
	}
	return decision, contextcompiler.StationReceipt{Calls: 1}, decision.ValidateFor(input)
}

func (station *scriptedConversationContextStation) SelectRelevant(
	_ context.Context,
	input assemblyline.ContextRelevanceInput,
) (assemblyline.ContextRelevanceDecision, contextcompiler.StationReceipt, error) {
	station.relevanceCalls++
	decision := assemblyline.ContextRelevanceDecision{
		Schema:                 assemblyline.ContextRelevanceSchemaV1,
		ReferencedCandidateIDs: append([]string{}, station.relevantIDs...),
	}
	return decision, contextcompiler.StationReceipt{Calls: 1}, decision.ValidateFor(input)
}

func (station *scriptedConversationContextStation) Minify(
	_ context.Context,
	input assemblyline.ContextMinificationInput,
) (assemblyline.ContextMinificationDecision, contextcompiler.StationReceipt, error) {
	station.minificationCalls++
	decision := assemblyline.ContextMinificationDecision{
		Schema:         assemblyline.ContextMinificationSchemaV1,
		MinimalContext: station.minimalContext,
	}
	return decision, contextcompiler.StationReceipt{Calls: 1}, decision.ValidateFor(input)
}

func emptyContextSieveStation() *scriptedConversationContextStation {
	return &scriptedConversationContextStation{terms: []string{}, relevantIDs: []string{}}
}

func answerObjectiveKindStation() *scriptedObjectiveKindStation {
	return &scriptedObjectiveKindStation{decision: assemblyline.ConversationObjectiveKindDecision{
		Schema: assemblyline.ConversationObjectiveKindSchemaV1,
		Kind:   assemblyline.ObjectiveKindAnswer,
	}}
}
